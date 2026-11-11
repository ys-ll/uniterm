package sync

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	ggittransport "github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/session"
	"github.com/ys-ll/uniterm/backend/utils"
)

var ErrWrongSyncPassword = errors.New("WRONG_SYNC_PASSWORD")

type SyncService struct {
	// configDir holds sync *metadata* — sync-config.json and the local
	// sync-repo clone. It is fixed to the OS user-config dir and is
	// independent of the migratable data directory.
	configDir string
	// dataDir is the resolved config data directory whose config files
	// (connections.json, ai.json, …) are encrypted to / decrypted
	// from the sync repo.
	dataDir     string
	repoPath    string
	keychain    *Keychain
	configStore *SyncConfigStore
	// passwordStore normalizes in-place encrypted fields (enc:v1:) to plaintext
	// on upload and back to enc:v1: on download. It is the credential store,
	// wired in after startup. nil until set; sync degrades to carrying fields
	// through opaque (best-effort) when it is absent.
	passwordStore PasswordStore
	// stores holds the config-exchange registry (SyncableStore), wired in
	// at startup. nil until set; sync then moves each store's blob through
	// ExportPlain/ImportPlain instead of touching per-file enc/dec itself.
	stores []SyncableStore
	mu     sync.Mutex

	// ready is closed by NewSyncService() once the disk-touching
	// init (UserConfigDir → MkdirAll → NewKeychain → NewSyncConfigStore)
	// has finished. Callers that arrive during the brief startup window
	// can wait on Ready() with a short timeout (F-407).
	ready     chan struct{}
	readyOnce sync.Once
}

type SyncResult struct {
	Direction SyncDirection `json:"direction"`
	Message   string        `json:"message"`
	Conflict  *ConflictInfo `json:"conflict,omitempty"`
}

type ConflictInfo struct {
	LocalTime  time.Time `json:"localTime"`
	RemoteTime time.Time `json:"remoteTime"`
}

func NewSyncService(dataDir string) (*SyncService, error) {
	metaDir, err := syncMetaDir()
	if err != nil {
		return nil, err
	}

	s := &SyncService{
		configDir:   metaDir,
		dataDir:     dataDir,
		repoPath:    filepath.Join(metaDir, "sync-repo"),
		keychain:    NewKeychain(),
		configStore: NewSyncConfigStore(metaDir),
		ready:       make(chan struct{}),
	}
	s.readyOnce.Do(func() { close(s.ready) })
	return s, nil
}

// NewSyncServiceAsync returns a SyncService whose disk-touching init
// (UserConfigDir / MkdirAll / NewKeychain) runs on a background
// goroutine. F-407: macOS Security framework keychain IPC + the
// PBKDF2 600k iterations probe can take 50–500ms+ on first launch —
// doing it synchronously inside wails.OnStartup blocks the first
// paint of the main window.
//
// Callers should `Ready()` (with a short timeout) before invoking
// service methods; otherwise methods may fail with
// ErrSyncNotInitialized until init completes.
func NewSyncServiceAsync(dataDir string) (*SyncService, context.Context) {
	s := &SyncService{ready: make(chan struct{})}
	initDone, cancel := context.WithCancel(context.Background())
	go func() {
		defer s.readyOnce.Do(func() { close(s.ready) })
		metaDir, err := syncMetaDir()
		if err != nil {
			return
		}
		s.configDir = metaDir
		s.dataDir = dataDir
		s.repoPath = filepath.Join(metaDir, "sync-repo")
		s.keychain = NewKeychain()
		s.configStore = NewSyncConfigStore(metaDir)
		cancel()
	}()
	return s, initDone
}

// Ready returns a channel closed once init has finished. Pair with
// a short timeout in callers that race startup (F-407).
func (s *SyncService) Ready() <-chan struct{} {
	return s.ready
}

// SetPasswordStore wires the credential store so sync can refuse to run on a
// locked vault. Field-level encryption is the stores' job now.
func (s *SyncService) SetPasswordStore(ps PasswordStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.passwordStore = ps
}

// SetSecretBackend swaps the keychain's backing storage. Server builds wire
// an encrypted-file store here: the container has no OS keychain (no Secret
// Service D-Bus), so every platform-keychain write would fail.
func (s *SyncService) SetSecretBackend(backend SecretStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.keychain != nil {
		s.keychain.backend = backend
	}
}

// SetStores wires the config-exchange registry (SyncableStore). Assembled
// in app.go — the only place that imports both sync and store. nil until
// set; sync then skips config exchange instead of touching files itself.
func (s *SyncService) SetStores(stores []SyncableStore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stores = stores
}

// newRemoteBackend builds the backend for a snapshot sync: one dedicated
// session per call (never the user's interactive one, never via the
// SessionManager), wrapped in the shared snapshot engine. The sessionFS
// provider parameters exist for tests; production passes the real
// constructors.
func (s *SyncService) newRemoteBackend(backend string, remote RemoteConfig, secret string, files []string) (SnapshotBackend, error) {
	switch backend {
	case "webdav", "":
		return newWebDAVSessionBackend(remote, secret, files, func() sessionFS { return session.NewWebDAVSession("sync-webdav") })
	case "sftp":
		return newSFTPBackend(remote, secret, files, func() sessionFS { return session.NewSFTPSession("sync-sftp") })
	case "s3":
		return newS3Backend(remote, secret, files, func() sessionFS { return session.NewS3Session("sync-s3") })
	case "smb":
		return newSMBBackend(remote, secret, files, func() sessionFS { return session.NewSMBSession("sync-smb") })
	default:
		return nil, fmt.Errorf("sync: unsupported backend %q", backend)
	}
}

// snapshotConfigured reports whether config carries a usable snapshot
// remote for its backend: webdav/sftp/smb need Server, s3 needs both
// Server (endpoint) and Bucket.
func snapshotConfigured(config SyncConfig) bool {
	switch config.Backend {
	case "webdav", "sftp", "smb":
		return config.Remote.Server != ""
	case "s3":
		return config.Remote.Server != "" && config.Remote.Bucket != ""
	default:
		return false
	}
}

// closeBackend disconnects a session-built remote backend. The session
// constructors dial eagerly and hold an open connection (SSH/SMB), so
// every call site that builds one through newRemoteBackend must defer
// this.
func closeBackend(b SnapshotBackend) {
	if c, ok := b.(io.Closer); ok {
		_ = c.Close()
	}
}

// requireUnlocked refuses sync when the credential store is wired but locked.
// A nil store means no credential store was wired (test-only path) and is not
// an error; production always wires it via SetPasswordStore. Callers must hold
// s.mu.
func (s *SyncService) requireUnlocked() error {
	if s.passwordStore == nil {
		return nil
	}
	if !s.passwordStore.Unlocked() {
		return utils.UserErr("vault_locked")
	}
	return nil
}

// GetConfig returns the current sync configuration.
func (s *SyncService) GetConfig() (SyncConfig, error) {
	return s.configStore.Load()
}

// SaveConfig persists sync configuration and stores the token if provided.
func (s *SyncService) SaveConfig(config SyncConfig, token string) error {
	if token != "" {
		if err := s.keychain.SetGitToken(token); err != nil {
			return fmt.Errorf("store token: %w", err)
		}
	}
	return s.configStore.Save(config)
}

func (s *SyncService) getToken() string {
	token, _ := s.keychain.GetGitToken()
	return token
}

// effectiveFiles returns this device's resolved sync file scope.
// A saved scope that resolves to nothing (e.g. all-invalid names from a
// stale build) falls back to the legacy default so a broken scope list
// can never turn sync into a silent no-op or a wholesale wipe.
func (s *SyncService) effectiveFiles() []string {
	config, err := s.configStore.Load()
	if err != nil {
		return EffectiveSyncFiles(nil)
	}
	files := EffectiveSyncFiles(config.SyncScope)
	if len(files) == 0 {
		return EffectiveSyncFiles(nil)
	}
	return files
}

// Sync runs a full sync cycle: clone/open → fetch → three-way content
// analysis → commit/pull/push → decrypt.
//
// The remote state is fetched BEFORE anything is committed locally: the
// old commit-then-fetch order turned "not pulled yet" into a local commit
// on a stale head, so two machines used alternately diverged and hit the
// conflict dialog on every open. Content is then analyzed three-way
// (local data dir vs merge base vs remote head) so that "merely out of
// date" is a silent pull and only a genuine both-sides edit surfaces the
// conflict dialog.
func (s *SyncService) Sync() (*SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}

	config, err := s.configStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if config.Backend != "" && config.Backend != "git" {
		if !snapshotConfigured(config) {
			return nil, fmt.Errorf("sync not configured: %s remote not set", config.Backend)
		}
		secret, _ := s.keychain.GetRemoteSecret(config.Backend)
		backend, err := s.newRemoteBackend(config.Backend, config.Remote, secret, nil)
		if err != nil {
			return nil, err
		}
		defer closeBackend(backend)
		return s.syncSnapshot(backend)
	}
	if config.RepoURL == "" {
		return nil, fmt.Errorf("sync not configured: repo URL not set")
	}

	encKey, err := s.keychain.GetEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}

	username := config.Username
	token := s.getToken()
	files := s.effectiveFiles()

	// 1. Clone or open repo.
	repo, err := CloneOrOpen(s.repoPath, config.RepoURL, config.Branch, username, token)
	if err != nil {
		s.updateLastSyncResult("failed", fmt.Sprintf("open repo: %v", err))
		return nil, fmt.Errorf("open repo: %w", err)
	}

	// SYNC-P1-11: a key that cannot open the existing ciphertext (wrong
	// master password, corrupted blob) must abort the sync before any
	// locally-derived ciphertext can be pushed over the only good copy.
	if repoHasFiles(s.repoPath) {
		if err := verifyDecryption(s.repoPath, encKey); err != nil {
			s.updateLastSyncResult("password_mismatch", err.Error())
			return nil, err
		}
	}

	// 2. Fetch on a truly empty remote returns ErrEmptyRemoteRepository —
	// treat as "remote has no branch yet", not as an error.
	if err := repo.Fetch(username, token); err != nil {
		if !errors.Is(err, ggittransport.ErrEmptyRemoteRepository) {
			s.updateLastSyncResult("failed", fmt.Sprintf("fetch: %v", err))
			return nil, fmt.Errorf("fetch: %w", err)
		}
	}

	heads, err := repo.ResolveBranchHeads(config.Branch)
	if err != nil {
		s.updateLastSyncResult("failed", fmt.Sprintf("resolve heads: %v", err))
		return nil, fmt.Errorf("resolve heads: %w", err)
	}

	// 3. Bootstrap cases.
	if heads.Local == nil && heads.Remote == nil {
		// Nothing anywhere: publish the local config as the initial commit.
		return s.pushLocalConfig(repo, encKey, username, token)
	}
	if heads.Local == nil {
		// Fresh local repo — adopt the remote silently.
		if err := repo.ResetToRemote(config.Branch); err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("reset: %v", err))
			return nil, fmt.Errorf("reset: %w", err)
		}
		if err := decryptStoresScoped(s.stores, files, s.repoPath, encKey); err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("decrypt files: %v", err))
			return nil, fmt.Errorf("decrypt files: %w", err)
		}
		s.updateLastSyncResult("success", "")
		return &SyncResult{Direction: SyncPull, Message: "配置已下载"}, nil
	}
	if heads.Remote == nil {
		// The remote branch does not exist (wiped or force-emptied) —
		// re-publish the local config on top of it.
		return s.pushLocalConfig(repo, encKey, username, token)
	}

	// 4. Heads in sync — only local data drift can require a push.
	if *heads.Local == *heads.Remote {
		same, err := s.dataMatchesCommit(files, repo, encKey, *heads.Local)
		if err != nil {
			s.updateLastSyncResult("failed", err.Error())
			return nil, err
		}
		if same {
			s.updateLastSyncResult("success", "")
			return &SyncResult{Message: "已是最新"}, nil
		}
		return s.pushLocalConfig(repo, encKey, username, token)
	}

	// 5. Heads diverged or fast-forwardable — three-way content analysis
	// against the merge base, so "only the remote moved" is a silent pull
	// and only a genuine both-sides edit surfaces the conflict dialog.
	base, err := repo.MergeBase(*heads.Local, *heads.Remote)
	if err != nil {
		s.updateLastSyncResult("failed", fmt.Sprintf("merge base: %v", err))
		return nil, fmt.Errorf("merge base: %w", err)
	}

	// History rewritten on another device (compaction force-push, manual
	// rewrite): no common ancestor exists. Compare the two sides' contents
	// directly — identical → silently re-anchor on the remote head;
	// different → the conflict dialog. Either way the device re-anchors
	// and shares history again afterwards.
	if base == nil {
		remoteDir, cleanupRemote, err := s.decryptCommitToDir(files, repo, encKey, heads.Remote)
		if err != nil {
			s.updateLastSyncResult("failed", err.Error())
			return nil, err
		}
		defer cleanupRemote()
		same, err := storesMatchDir(s.stores, files, remoteDir)
		if err != nil {
			s.updateLastSyncResult("failed", err.Error())
			return nil, err
		}
		if same {
			if err := repo.ResetToRemote(config.Branch); err != nil {
				s.updateLastSyncResult("failed", fmt.Sprintf("reset: %v", err))
				return nil, fmt.Errorf("reset: %w", err)
			}
			s.updateLastSyncResult("success", "")
			return &SyncResult{Message: "已是最新"}, nil
		}
		localTime, err := repo.CommitTime(*heads.Local)
		if err != nil {
			s.updateLastSyncResult("failed", err.Error())
			return nil, err
		}
		remoteTime, err := repo.CommitTime(*heads.Remote)
		if err != nil {
			s.updateLastSyncResult("failed", err.Error())
			return nil, err
		}
		s.updateLastSyncResult("conflict", "")
		return &SyncResult{
			Direction: SyncConflict,
			Conflict:  &ConflictInfo{LocalTime: localTime, RemoteTime: remoteTime},
		}, nil
	}

	baseDir, cleanupBase, err := s.decryptCommitToDir(files, repo, encKey, base)
	if err != nil {
		s.updateLastSyncResult("failed", err.Error())
		return nil, err
	}
	defer cleanupBase()
	remoteDir, cleanupRemote, err := s.decryptCommitToDir(files, repo, encKey, heads.Remote)
	if err != nil {
		s.updateLastSyncResult("failed", err.Error())
		return nil, err
	}
	defer cleanupRemote()

	// The local side is the store's canonical export; base/remote are
	// decrypted commit copies, so both sides are plaintext here.
	localSame, err := storesMatchDir(s.stores, files, baseDir)
	if err != nil {
		s.updateLastSyncResult("failed", err.Error())
		return nil, err
	}
	localChanged := !localSame
	remoteChanged, err := dirsDiffer(files, remoteDir, baseDir)
	if err != nil {
		s.updateLastSyncResult("failed", err.Error())
		return nil, err
	}

	switch {
	case !localChanged && !remoteChanged:
		// Content identical on both sides, heads diverged anyway (e.g.
		// re-encrypt noise from older builds) — heal the history
		// silently; no data changes.
		if err := repo.ResetToRemote(config.Branch); err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("reset: %v", err))
			return nil, fmt.Errorf("reset: %w", err)
		}
		s.updateLastSyncResult("success", "")
		return &SyncResult{Message: "已是最新"}, nil

	case localChanged && !remoteChanged:
		// Only the local content is newer. Re-anchor the clone on the
		// remote head and publish the local content as one commit on top
		// of it, so the push is a fast-forward (any unpushed local
		// commits are squashed — they were never shared).
		if err := repo.ResetToRemote(config.Branch); err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("reset: %v", err))
			return nil, fmt.Errorf("reset: %w", err)
		}
		return s.pushLocalConfig(repo, encKey, username, token)

	case !localChanged && remoteChanged:
		// Only the remote moved — silent pull.
		if err := repo.ResetToRemote(config.Branch); err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("reset: %v", err))
			return nil, fmt.Errorf("reset: %w", err)
		}
		if err := decryptStoresScoped(s.stores, files, s.repoPath, encKey); err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("decrypt files: %v", err))
			return nil, fmt.Errorf("decrypt files: %w", err)
		}
		s.updateLastSyncResult("success", "")
		return &SyncResult{Direction: SyncPull, Message: "配置已下载"}, nil

	default:
		// Both sides changed content since the merge base — a genuine
		// conflict; let the user pick a direction.
		localTime, err := repo.CommitTime(*heads.Local)
		if err != nil {
			s.updateLastSyncResult("failed", err.Error())
			return nil, err
		}
		remoteTime, err := repo.CommitTime(*heads.Remote)
		if err != nil {
			s.updateLastSyncResult("failed", err.Error())
			return nil, err
		}
		s.updateLastSyncResult("conflict", "")
		return &SyncResult{
			Direction: SyncConflict,
			Conflict: &ConflictInfo{
				LocalTime:  localTime,
				RemoteTime: remoteTime,
			},
		}, nil
	}
}

const (
	// compactionThreshold is the first-parent depth at which sync history
	// is rewritten down to compactionKeep commits. Encrypted config blobs
	// do not delta-compress, so every commit stores a near-full copy of
	// all files and repo size grows as ≈ commits × full-size.
	compactionThreshold = 50
	// compactionKeep is how many recent commits survive a rewrite.
	compactionKeep = 10
)

// maybeCompactHistory bounds the sync repo size: once the first-parent
// depth reaches compactionThreshold, the remote history is rewritten down
// to the most recent compactionKeep commits, force-pushed, and the local
// clone is rebuilt (go-git cannot gc old objects, so only a fresh clone
// sheds them).
//
// Best-effort by contract: any failure logs and skips the round —
// compaction never fails or blocks the sync that triggered it. Callers
// must hold s.mu.
func (s *SyncService) maybeCompactHistory() {
	config, err := s.configStore.Load()
	if err != nil || config.RepoURL == "" {
		return
	}
	username := config.Username
	token := s.getToken()

	repo, err := CloneOrOpen(s.repoPath, config.RepoURL, config.Branch, username, token)
	if err != nil {
		log.Writef("sync compaction: open repo: %v", err)
		return
	}
	depth, err := repo.CommitDepth()
	if err != nil {
		log.Writef("sync compaction: depth: %v", err)
		return
	}
	if depth < compactionThreshold {
		return
	}
	// The force-push below replaces remote history wholesale. If another
	// device pushed between our own push and this rewrite, force-pushing
	// would silently orphan its commit and surface a spurious conflict
	// there. Right after a successful push the two heads must be equal —
	// anything else means the remote moved, so skip the round.
	if err := repo.Fetch(username, token); err != nil && !errors.Is(err, ggittransport.ErrEmptyRemoteRepository) {
		log.Writef("sync compaction: fetch: %v", err)
		return
	}
	heads, err := repo.ResolveBranchHeads(config.Branch)
	if err != nil {
		log.Writef("sync compaction: resolve heads: %v", err)
		return
	}
	if heads.Local == nil || heads.Remote == nil || *heads.Local != *heads.Remote {
		log.Writef("sync compaction: remote head moved since push, skipping round")
		return
	}
	// Remember the pre-rewrite head: if the force-push below fails, the
	// branch must be restored, or local and remote are half-rewritten —
	// no common ancestor — and the next sync surfaces a spurious conflict.
	prevHead, err := repo.HeadRef()
	if err != nil {
		log.Writef("sync compaction: head: %v", err)
		return
	}
	if err := repo.RewriteHistory(compactionKeep); err != nil {
		log.Writef("sync compaction: rewrite: %v", err)
		return
	}
	if err := repo.PushForce(username, token); err != nil {
		log.Writef("sync compaction: force push: %v", err)
		if refErr := repo.RestoreHead(prevHead.Name(), prevHead.Hash()); refErr != nil {
			log.Writef("sync compaction: restore head: %v", refErr)
		}
		return
	}
	// Rebuild the clone from scratch — the only way to shed the old
	// objects; go-git cannot prune them.
	if err := os.RemoveAll(s.repoPath); err != nil {
		log.Writef("sync compaction: remove clone: %v", err)
		return
	}
	if _, err := CloneOrOpen(s.repoPath, config.RepoURL, config.Branch, username, token); err != nil {
		log.Writef("sync compaction: re-clone: %v", err)
		return
	}
	log.Writef("sync compaction: depth %d → %d commits", depth, compactionKeep)
}

// pushLocalConfig encrypts the local config into the repo, commits it on
// top of the current HEAD and pushes.
func (s *SyncService) pushLocalConfig(repo *GitRepo, encKey []byte, username, token string) (*SyncResult, error) {
	files := s.effectiveFiles()
	if err := encryptStoresScoped(s.stores, files, s.repoPath, encKey, nil); err != nil {
		s.updateLastSyncResult("failed", fmt.Sprintf("encrypt files: %v", err))
		return nil, fmt.Errorf("encrypt files: %w", err)
	}
	if _, err := repo.StageAndCommitFiles(commitMsg("uniTerm config sync"), files); err != nil {
		s.updateLastSyncResult("failed", fmt.Sprintf("commit: %v", err))
		return nil, fmt.Errorf("commit: %w", err)
	}
	if err := repo.Push(username, token); err != nil {
		s.updateLastSyncResult("failed", fmt.Sprintf("push: %v", err))
		return nil, fmt.Errorf("push: %w", err)
	}
	s.updateLastSyncResult("success", "")
	s.maybeCompactHistory()
	return &SyncResult{Direction: SyncPush, Message: "配置已上传"}, nil
}

// dataMatchesCommit compares the local config dir with the config as
// committed at hash over the given files. A decrypt error is surfaced, not
// swallowed (SYNC-P1-11).
func (s *SyncService) dataMatchesCommit(files []string, repo *GitRepo, encKey []byte, hash plumbing.Hash) (bool, error) {
	dir, cleanup, err := s.decryptCommitToDir(files, repo, encKey, &hash)
	if err != nil {
		return false, err
	}
	defer cleanup()
	same, err := storesMatchDir(s.stores, files, dir)
	if err != nil {
		return false, err
	}
	return same, nil
}

// decryptCommitToDir extracts the given files as committed at hash and
// decrypts them into a fresh temp dir. A nil hash yields an empty dir
// (nothing committed at that side). The caller owns cleanup via the
// returned function.
func (s *SyncService) decryptCommitToDir(files []string, repo *GitRepo, encKey []byte, hash *plumbing.Hash) (string, func(), error) {
	cipherDir, err := os.MkdirTemp("", "sync-cipher-")
	if err != nil {
		return "", nil, err
	}
	plainDir, err := os.MkdirTemp("", "sync-plain-")
	if err != nil {
		os.RemoveAll(cipherDir)
		return "", nil, err
	}
	cleanup := func() {
		os.RemoveAll(cipherDir)
		os.RemoveAll(plainDir)
	}
	if hash != nil {
		if err := repo.ExtractCommitFilesIn(*hash, files, cipherDir); err != nil {
			cleanup()
			return "", nil, err
		}
		if err := DecryptConfigFilesScoped(files, cipherDir, plainDir, encKey); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("decrypt commit: %w", err)
		}
	}
	return plainDir, cleanup, nil
}

// dirsDiffer reports whether the two decrypted config directories differ
// over the given files.
func dirsDiffer(files []string, a, b string) (bool, error) {
	same, err := compareConfigDirs(files, a, b)
	if err != nil {
		return false, err
	}
	return !same, nil
}

// ResolveConflict handles a conflict by forcing push or reset.
func (s *SyncService) ResolveConflict(useLocal bool) (*SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}

	config, err := s.configStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if config.Backend != "" && config.Backend != "git" {
		if !snapshotConfigured(config) {
			return nil, fmt.Errorf("sync not configured: %s remote not set", config.Backend)
		}
		secret, _ := s.keychain.GetRemoteSecret(config.Backend)
		backend, err := s.newRemoteBackend(config.Backend, config.Remote, secret, nil)
		if err != nil {
			return nil, err
		}
		defer closeBackend(backend)
		return s.resolveConflictSnapshot(backend, useLocal)
	}
	username := config.Username
	token := s.getToken()
	files := s.effectiveFiles()

	repo, err := CloneOrOpen(s.repoPath, config.RepoURL, config.Branch, username, token)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	encKey, err := s.keychain.GetEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}

	// SYNC-P1-11: same guard as Sync — a key that cannot open the
	// existing ciphertext must abort before anything is pushed.
	if repoHasFiles(s.repoPath) {
		if err := verifyDecryption(s.repoPath, encKey); err != nil {
			s.updateLastSyncResult("password_mismatch", err.Error())
			return nil, err
		}
	}

	// Learn the remote state first — same ordering rule as Sync.
	if err := repo.Fetch(username, token); err != nil {
		if !errors.Is(err, ggittransport.ErrEmptyRemoteRepository) {
			return nil, fmt.Errorf("fetch: %w", err)
		}
	}
	heads, err := repo.ResolveBranchHeads(config.Branch)
	if err != nil {
		return nil, fmt.Errorf("resolve heads: %w", err)
	}

	if useLocal {
		// Re-anchor the clone on the remote head and publish the local
		// content as one commit on top of it. The push is then a
		// fast-forward — force-pushing here orphaned the other machines'
		// heads and made them conflict again on their next open, creating
		// an endless resolve-conflict ping-pong.
		if heads.Remote != nil {
			if err := repo.ResetToRemote(config.Branch); err != nil {
				return nil, fmt.Errorf("reset: %w", err)
			}
		}
		if err := encryptStoresScoped(s.stores, files, s.repoPath, encKey, nil); err != nil {
			return nil, fmt.Errorf("encrypt files: %w", err)
		}
		if _, err := repo.StageAndCommitFiles(commitMsg("uniTerm config sync (resolve conflict)"), files); err != nil {
			return nil, fmt.Errorf("commit: %w", err)
		}
		if err := repo.Push(username, token); err != nil {
			return nil, fmt.Errorf("push: %w", err)
		}
		s.updateLastSyncResult("success", "")
		s.maybeCompactHistory()
		return &SyncResult{Direction: SyncPush, Message: "已用本地配置覆盖远端"}, nil
	}

	if heads.Remote != nil {
		if err := repo.ResetToRemote(config.Branch); err != nil {
			return nil, fmt.Errorf("reset: %w", err)
		}
	}
	if err := decryptStoresScoped(s.stores, files, s.repoPath, encKey); err != nil {
		return nil, fmt.Errorf("decrypt files: %w", err)
	}

	s.updateLastSyncResult("success", "")
	return &SyncResult{Direction: SyncPull, Message: "已用远端配置覆盖本地"}, nil
}

// TestConnection verifies the sync backend is reachable with stored credentials.
func (s *SyncService) TestConnection() error {
	config, err := s.configStore.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if config.Backend != "" && config.Backend != "git" {
		if !snapshotConfigured(config) {
			return utils.UserErr("repo_not_configured")
		}
		secret, _ := s.keychain.GetRemoteSecret(config.Backend)
		backend, err := s.newRemoteBackend(config.Backend, config.Remote, secret, nil)
		if err != nil {
			return err
		}
		defer closeBackend(backend)
		return backend.Probe()
	}
	if config.RepoURL == "" {
		return utils.UserErr("repo_not_configured")
	}
	username := config.Username
	token := s.getToken()
	return TestConnection(config.RepoURL, username, token)
}

// TestRemote probes a snapshot remote with form-provided values, without
// touching the stored configuration — the add-repo dialog's test button.
// Unlike TestConnection it never reads sync-config.json, so it can verify
// a candidate remote before the user commits to it.
func (s *SyncService) TestRemote(backend string, remote RemoteConfig, secret string) error {
	b, err := s.newRemoteBackend(backend, remote, secret, nil)
	if err != nil {
		return err
	}
	defer closeBackend(b)
	return b.Probe()
}

func (s *SyncService) updateLastSyncResult(status string, errMsg string) {
	config, _ := s.configStore.Load()
	config.LastSyncAt = time.Now()
	config.LastSyncStatus = status
	config.LastSyncError = errMsg
	_ = s.configStore.Save(config)
}

// IsAutoSyncEnabled returns whether auto sync is enabled and configured
// for the active backend (git repo URL, or a WebDAV server in webdav mode).
func (s *SyncService) IsAutoSyncEnabled() bool {
	config, _ := s.configStore.Load()
	return config.AutoSync && (config.RepoURL != "" || snapshotConfigured(config))
}

// RepoPath returns the local git repo path.
func (s *SyncService) RepoPath() string {
	return s.repoPath
}

// PasswordStore returns the keychain as a PasswordStore for connection store integration.
func (s *SyncService) PasswordStore() *Keychain {
	return s.keychain
}

// ConfigureRepo sets up a new or existing sync repository.
func (s *SyncService) ConfigureRepo(repoURL, username, token, masterPassword string) (*SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}

	repo, err := CloneOrOpen(s.repoPath, repoURL, "main", username, token)
	if err != nil {
		return nil, fmt.Errorf("clone/open repo: %w", err)
	}
	files := s.effectiveFiles()

	// Check if remote has .sync-salt
	salt, err := ReadSaltFile(s.repoPath)
	if err != nil {
		return nil, fmt.Errorf("read salt: %w", err)
	}

	var encKey []byte
	if salt != nil {
		// Existing repo: verify password, then compare remote vs local
		encKey = DeriveKey(masterPassword, salt)
		if err := verifyDecryption(s.repoPath, encKey); err != nil {
			return nil, utils.UserErr("master_password_mismatch")
		}

		if err := s.keychain.StoreEncryptionKey(encKey); err != nil {
			return nil, fmt.Errorf("store encryption key: %w", err)
		}

		// Decrypt remote to temp dir for comparison
		tmpDir, err := os.MkdirTemp("", "sync-compare")
		if err != nil {
			return nil, fmt.Errorf("create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		if err := DecryptConfigFilesScoped(files, s.repoPath, tmpDir, encKey); err != nil {
			return nil, fmt.Errorf("decrypt remote for comparison: %w", err)
		}

		localEmpty := isConfigDirEmpty(files, s.dataDir)
		remoteEmpty := isConfigDirEmpty(files, tmpDir)

		if localEmpty {
			// Local has no config — pull remote to local
			if err := decryptStoresScoped(s.stores, files, s.repoPath, encKey); err != nil {
				return nil, fmt.Errorf("decrypt files: %w", err)
			}
			// Carry over device-local fields: re-configuring the repo must
			// not wipe this device's sync scope or auto-sync setting.
			prev, _ := s.configStore.Load()
			cfg := SyncConfig{
				RepoURL:   repoURL,
				Branch:    "main",
				Username:  username,
				AutoSync:  prev.AutoSync,
				SyncScope: prev.SyncScope,
			}
			if token != "" {
				_ = s.keychain.SetGitToken(token)
			}
			_ = s.configStore.Save(cfg)
			s.updateLastSyncResult("success", "")
			return &SyncResult{Direction: SyncPull, Message: "仓库配置成功，已从远端同步配置"}, nil
		}
		if !remoteEmpty {
			// Both have data — compare
			same, err := storesMatchDir(s.stores, files, tmpDir)
			if err != nil {
				return nil, fmt.Errorf("compare configs: %w", err)
			}
			if !same {
				// Carry over device-local fields: re-configuring the repo
				// must not wipe this device's sync scope or auto-sync setting.
				prev, _ := s.configStore.Load()
				cfg := SyncConfig{
					RepoURL:   repoURL,
					Branch:    "main",
					Username:  username,
					AutoSync:  prev.AutoSync,
					SyncScope: prev.SyncScope,
				}
				if token != "" {
					_ = s.keychain.SetGitToken(token)
				}
				_ = s.configStore.Save(cfg)
				localTime := getConfigModTime(s.dataDir)
				remoteTime := getConfigModTime(tmpDir)
				s.updateLastSyncResult("conflict", "")
				return &SyncResult{
					Direction: SyncConflict,
					Message:   "本地和远端配置不一致，请选择覆盖方向",
					Conflict: &ConflictInfo{
						LocalTime:  localTime,
						RemoteTime: remoteTime,
					},
				}, nil
			}
			// Same — save config then return, no need to re-encrypt/push
			// Carry over device-local fields: re-configuring the repo must
			// not wipe this device's sync scope or auto-sync setting.
			prev, _ := s.configStore.Load()
			cfg := SyncConfig{
				RepoURL:   repoURL,
				Branch:    "main",
				Username:  username,
				AutoSync:  prev.AutoSync,
				SyncScope: prev.SyncScope,
			}
			if token != "" {
				_ = s.keychain.SetGitToken(token)
			}
			_ = s.configStore.Save(cfg)
			if err := decryptStoresScoped(s.stores, files, s.repoPath, encKey); err != nil {
				return nil, fmt.Errorf("decrypt files: %w", err)
			}
			s.updateLastSyncResult("success", "")
			return &SyncResult{Message: "仓库配置成功"}, nil
		}
		// Remote is empty — fall through to encrypt and push below
	} else {
		// New repo: generate salt, derive key
		salt, err = GenerateSalt()
		if err != nil {
			return nil, fmt.Errorf("generate salt: %w", err)
		}
		encKey = DeriveKey(masterPassword, salt)
		if err := WriteSaltFile(s.repoPath, salt); err != nil {
			return nil, fmt.Errorf("write salt: %w", err)
		}

		if err := s.keychain.StoreEncryptionKey(encKey); err != nil {
			return nil, fmt.Errorf("store encryption key: %w", err)
		}
	}

	// Encrypt and push local config
	if err := encryptStoresScoped(s.stores, files, s.repoPath, encKey, nil); err != nil {
		return nil, fmt.Errorf("encrypt files: %w", err)
	}

	if _, err := repo.StageAndCommitFiles(commitMsg("uniTerm config sync"), files); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	if err := repo.PushToBranch("main", username, token); err != nil {
		// Pull first then push
		if pullErr := repo.Pull(username, token); pullErr == nil {
			if pushErr := repo.PushToBranch("main", username, token); pushErr != nil {
				return nil, fmt.Errorf("push: %w", pushErr)
			}
		} else {
			return nil, fmt.Errorf("push: %w", err)
		}
	}

	// Carry over device-local fields: re-configuring the repo must not
	// wipe this device's sync scope or auto-sync setting.
	prev, _ := s.configStore.Load()
	cfg := SyncConfig{
		RepoURL:   repoURL,
		Branch:    "main",
		Username:  username,
		AutoSync:  prev.AutoSync,
		SyncScope: prev.SyncScope,
	}
	if token != "" {
		if err := s.keychain.SetGitToken(token); err != nil {
			return nil, fmt.Errorf("store token: %w", err)
		}
	}
	if err := s.configStore.Save(cfg); err != nil {
		return nil, fmt.Errorf("save config: %w", err)
	}

	// Decrypt remote files to local
	if err := decryptStoresScoped(s.stores, files, s.repoPath, encKey); err != nil {
		return nil, fmt.Errorf("decrypt files: %w", err)
	}

	// Compaction deletes and re-clones s.repoPath best-effort, so it must
	// run after the last use of that directory in this function.
	s.maybeCompactHistory()

	s.updateLastSyncResult("success", "")
	return &SyncResult{Direction: SyncPush, Message: "仓库配置成功"}, nil
}

// getConfigModTime returns the latest modification time of config files in a directory.
func getConfigModTime(dir string) time.Time {
	var latest time.Time
	for _, name := range []string{"connections.json", "settings.json", "quickCommands.json", "tunnels.json", "identities.json", "proxies.json"} {
		info, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	return latest
}

// isConfigDirEmpty returns true if the config dir has no meaningful data
// over the given files. Counts every file in the list — not only connections —
// so a user with settings
// / quick-commands but no connections is not treated as "empty" and silently
// overwritten on first sync (SYNC-P0-1). Uses the given file list rather than
// every persisted JSON: ai-sessions.json / skills.json are local-only and never
// synced, so their presence must not block a first-sync add-and-pull
// (their files are not touched by decrypt either). favorites.json is an
// array file: only a non-empty array counts as data.
func isConfigDirEmpty(files []string, dir string) bool {
	for _, name := range files {
		v, err := readJSONValue(filepath.Join(dir, name))
		if err != nil {
			// Unparseable or unreadable — treat as non-empty so the
			// user's data is never silently nuked.
			return false
		}
		if v != nil {
			return false
		}
	}
	return true
}

// compareConfigDirs compares two decrypted config directories over the given
// files. Both sides are already plaintext (a decrypted remote copy against
// another), so no field-level normalization applies; the local store's
// plaintext is compared through storesMatchDir instead.
func compareConfigDirs(files []string, a, b string) (bool, error) {
	for _, name := range files {
		same, err := compareConfigFiles(filepath.Join(a, name), filepath.Join(b, name))
		if err != nil {
			return false, err
		}
		if !same {
			return false, nil
		}
	}
	return true, nil
}

// readJSONValue decodes a config file into a generic JSON value. Most synced
// files are objects, but favorites.json is a top-level array, so the value
// must not be assumed to be a map. A missing file and explicitly empty
// wrappers ({}, [], null) all normalize to nil — they all mean "no data" and
// must compare equal to each other.
func readJSONValue(path string) (interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return jsonValue(data)
}

// jsonValue normalizes a JSON document to a comparable value: blank input
// and the explicitly empty wrappers ({}, [], null) all mean "no data" and
// must compare equal to each other.
func jsonValue(raw []byte) (interface{}, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	switch t := v.(type) {
	case map[string]interface{}:
		if len(t) == 0 {
			return nil, nil
		}
	case []interface{}:
		if len(t) == 0 {
			return nil, nil
		}
	}
	return v, nil
}

// canonicalJSON renders raw JSON in the canonical comparable form: empty
// wrappers collapse to null and encoding/json sorts map keys, so a store's
// struct-order marshal and a remote blob's map-order marshal produce
// identical bytes for equal content.
func canonicalJSON(raw []byte) (string, error) {
	v, err := jsonValue(raw)
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// compareConfigFiles compares two config files as canonical JSON. Array
// files (favorites.json) keep their order, which is meaningful. Both sides
// are decrypted plaintext by the time they get here — the local store's
// plaintext never enters this path; storesMatchDir covers the
// local-store-vs-remote direction.
func compareConfigFiles(aPath, bPath string) (bool, error) {
	aVal, err := readJSONValue(aPath)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", aPath, err)
	}
	bVal, err := readJSONValue(bPath)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", bPath, err)
	}
	aNorm, err := json.MarshalIndent(aVal, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal %s: %w", aPath, err)
	}
	bNorm, err := json.MarshalIndent(bVal, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal %s: %w", bPath, err)
	}
	return string(aNorm) == string(bNorm), nil
}

// storesMatchDir reports whether every registry store (restricted to files)
// exports content equal to the decrypted remote blob in dir. This is the
// local-vs-remote comparison: the store is the single source of truth for
// the local side, so no keychain backfill or enc:v1: normalization happens
// here — ExportPlain already applied the store's full semantics. A store
// whose backing file was never written and a blob the remote never carried
// both read as absent, and absent matches absent.
func storesMatchDir(stores []SyncableStore, files []string, dir string) (bool, error) {
	if err := requireStores(stores, files); err != nil {
		return false, err
	}
	wanted := make(map[string]bool, len(files))
	for _, name := range files {
		wanted[name] = true
	}
	for _, st := range stores {
		name := st.BlobName()
		if !wanted[name] {
			continue
		}
		var local []byte
		plain, err := st.ExportPlain()
		switch {
		case err == nil:
			local = plain
		case errors.Is(err, fs.ErrNotExist):
			// Never persisted: compares as absent.
		default:
			return false, fmt.Errorf("export %s: %w", name, err)
		}
		remote, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil && !os.IsNotExist(err) {
			return false, fmt.Errorf("read %s: %w", name, err)
		}
		if os.IsNotExist(err) {
			remote = nil
		}
		localNorm, err := canonicalJSON(local)
		if err != nil {
			return false, fmt.Errorf("parse local %s: %w", name, err)
		}
		remoteNorm, err := canonicalJSON(remote)
		if err != nil {
			return false, fmt.Errorf("parse remote %s: %w", name, err)
		}
		if localNorm != remoteNorm {
			return false, nil
		}
	}
	return true, nil
}

// VerifySyncPassword validates credentials against the remote and verifies the
// password can decrypt the remote config. username and token are the new values
// from the form; token may be empty to keep the stored one.
func (s *SyncService) VerifySyncPassword(password, username, token string) error {
	config, err := s.configStore.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if config.RepoURL == "" {
		return fmt.Errorf("no repo configured")
	}

	if token == "" {
		token = s.getToken()
	}

	// 1. Verify the new credentials can reach the remote
	if err := TestConnection(config.RepoURL, username, token); err != nil {
		return fmt.Errorf("cannot reach remote: %w", err)
	}

	// 2. Open repo and fetch latest encrypted files with validated credentials
	repo, err := CloneOrOpen(s.repoPath, config.RepoURL, config.Branch, username, token)
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}
	_ = repo.Fetch(username, token)

	// 3. Derive or load key and verify it can decrypt the remote config
	salt, err := ReadSaltFile(s.repoPath)
	if err != nil {
		return fmt.Errorf("read salt: %w", err)
	}
	if salt == nil {
		return utils.UserErr("salt_missing")
	}

	var key []byte
	if password == "" {
		key, err = s.keychain.GetEncryptionKey()
		if err != nil {
			return ErrWrongSyncPassword
		}
	} else {
		key = DeriveKey(password, salt)
	}

	// Try each scoped file on the remote, then the local mirror — the
	// scope may legitimately exclude any specific file.
	files := s.effectiveFiles()
	var encrypted []byte
	for _, name := range files {
		data, rerr := repo.ReadRemoteFile(config.Branch, name)
		if rerr == nil && len(data) > 0 {
			encrypted = data
			break
		}
	}
	if encrypted == nil {
		for _, name := range files {
			data, rerr := os.ReadFile(filepath.Join(s.repoPath, name))
			if rerr == nil && len(data) > 0 {
				encrypted = data
				break
			}
		}
	}
	if encrypted == nil {
		// No scoped ciphertext on the remote or local mirror — an empty
		// repo also lands here (pre-existing semantics).
		return ErrWrongSyncPassword
	}

	if _, err := decryptBytes(string(encrypted), key); err != nil {
		return ErrWrongSyncPassword
	}
	return nil
}

// ChangePassword re-encrypts every ciphertext file present in the repo
// with a new master password and a fresh random salt. The rotation is
// repo-global because the salt is repo-global: every device derives its
// key from the same salt, so no ciphertext may survive under the old key.
// A new salt is required so PBKDF2 work cannot be
// amortized across old and new passwords (SYNC-P1-6). The repo's existing
// ciphertext is decrypted with the old key and re-encrypted with the new
// key via a temp-then-rename pattern, so a crash mid-rotation leaves the
// repo decryptable with the old key.
func (s *SyncService) ChangePassword(oldPassword, newPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	config, err := s.configStore.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if config.Backend == "webdav" {
		return s.changePasswordSnapshot(oldPassword, newPassword)
	}
	if config.RepoURL == "" {
		return fmt.Errorf("no repo configured")
	}

	oldSalt, err := ReadSaltFile(s.repoPath)
	if err != nil {
		return fmt.Errorf("read salt: %w", err)
	}
	if oldSalt == nil {
		return utils.UserErr("salt_missing")
	}

	// Verify old password can decrypt the repo.
	oldKey := DeriveKey(oldPassword, oldSalt)
	if err := verifyDecryption(s.repoPath, oldKey); err != nil {
		return utils.UserErr("current_password_mismatch")
	}

	// Fresh 16-byte salt via crypto/rand (SYNC-P1-6).
	newSalt, err := GenerateSalt()
	if err != nil {
		return fmt.Errorf("generate new salt: %w", err)
	}
	newKey := DeriveKey(newPassword, newSalt)
	if err := s.keychain.StoreEncryptionKey(newKey); err != nil {
		return fmt.Errorf("store new encryption key: %w", err)
	}

	// Rotate every ciphertext present in the repo, not just the local
	// scope: the new salt changes every device's derived key, so any
	// file left under the old key would permanently break their sync.
	var files []string
	for _, name := range syncableFiles {
		if _, err := os.Stat(filepath.Join(s.repoPath, name)); err == nil {
			files = append(files, name)
		}
	}

	// Re-encrypt every existing repo ciphertext: decrypt with oldKey,
	// re-encrypt with newKey, atomic rename. A crash before the rename
	// leaves the original ciphertext intact and decryptable with oldKey.
	for _, name := range files {
		srcPath := filepath.Join(s.repoPath, name)
		ciphertext, err := os.ReadFile(srcPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("read %s: %w", name, err)
		}
		plaintext, err := decryptBytes(string(ciphertext), oldKey)
		if err != nil {
			return fmt.Errorf("decrypt %s: %w", name, err)
		}
		encoded, err := encryptBytes(plaintext, newKey)
		if err != nil {
			return fmt.Errorf("encrypt %s: %w", name, err)
		}
		tmpPath := srcPath + ".tmp"
		if err := os.WriteFile(tmpPath, []byte(encoded), 0600); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("write %s.tmp: %w", name, err)
		}
		if err := os.Rename(tmpPath, srcPath); err != nil {
			os.Remove(tmpPath)
			return fmt.Errorf("rename %s: %w", name, err)
		}
	}

	// Atomic swap of .sync-salt.
	saltPath := filepath.Join(s.repoPath, ".sync-salt")
	saltTmp := saltPath + ".tmp"
	if err := os.WriteFile(saltTmp, []byte(hex.EncodeToString(newSalt)), 0600); err != nil {
		os.Remove(saltTmp)
		return fmt.Errorf("write .sync-salt.tmp: %w", err)
	}
	if err := os.Rename(saltTmp, saltPath); err != nil {
		os.Remove(saltTmp)
		return fmt.Errorf("rename .sync-salt: %w", err)
	}

	username := config.Username
	token := s.getToken()

	repo, err := CloneOrOpen(s.repoPath, config.RepoURL, config.Branch, username, token)
	if err != nil {
		return fmt.Errorf("open repo: %w", err)
	}

	if _, err := repo.StageAndCommitFiles(commitMsg("uniTerm config sync (change password)"), files); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if err := repo.Push(username, token); err != nil {
		return fmt.Errorf("push: %w", err)
	}

	s.maybeCompactHistory()

	return nil
}

// DeleteRepo removes the local sync repo and credentials.
func (s *SyncService) DeleteRepo() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.RemoveAll(s.repoPath); err != nil {
		return fmt.Errorf("remove repo: %w", err)
	}

	_ = s.keychain.Delete("encryption-key")
	_ = s.keychain.Delete("git-token")
	_ = s.keychain.Delete("webdav-password")
	_ = s.keychain.DeleteRemoteSecret("sftp")
	_ = s.keychain.DeleteRemoteSecret("s3")
	_ = s.keychain.DeleteRemoteSecret("smb")

	return s.configStore.Save(SyncConfig{Branch: "main"})
}

// commitMsg builds a commit message. Hostname and user identity are
// intentionally NOT included — the sync repo is often shared across
// devices and machines, and leaking os.Hostname() to a public-by-mistake
// repo is a privacy regression (SYNC-P1-4).
func commitMsg(action string) string {
	return fmt.Sprintf("%s | %s", action, time.Now().Format(time.RFC3339))
}

// repoHasFiles reports whether the repo carries any encrypted config blob.
// It is the "is there a remote copy to protect" heuristic, so ANY blob
// counts: sync skips a store whose backing file was never written, so a real
// repo routinely carries a subset of the synced files. Requiring all of them
// would silently disable the wrong-key guard below on most repos.
func repoHasFiles(repoPath string) bool {
	for _, name := range defaultSyncedFiles {
		if _, err := os.Stat(filepath.Join(repoPath, name)); err == nil {
			return true
		}
	}
	return false
}

// verifyDecryption checks that the given key can decrypt every config blob
// the repo carries — not just connections.json, which a scoped repo may
// legitimately lack. DecryptConfigFiles skips blobs that are absent.
func verifyDecryption(repoPath string, key []byte) error {
	if !repoHasFiles(repoPath) {
		return nil
	}
	tmpDir, err := os.MkdirTemp("", "sync-verify")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	return DecryptConfigFiles(repoPath, tmpDir, key)
}
