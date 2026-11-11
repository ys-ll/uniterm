package sync

import (
	"fmt"
	"os"

	"github.com/ys-ll/uniterm/backend/utils"
)

// ConfigureRepoRemote sets up (or adopts) a snapshot remote for any of the
// snapshot backends (webdav/sftp/s3/smb). An existing remote snapshot is
// adopted: its salt is the repo identity, the master password is verified
// against its ciphertext, and the local side either pulls (no local data),
// records the agreed hash (identical), or surfaces the conflict dialog
// (both sides have data). A fresh remote generates a new salt and
// publishes the local config as the initial snapshot.
func (s *SyncService) ConfigureRepoRemote(backend string, remote RemoteConfig, secret, masterPassword string) (*SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireUnlocked(); err != nil {
		return nil, err
	}

	// Carry over this device's existing scope/auto-sync (empty on first
	// configure → legacy default set). Every config-save site must
	// preserve AutoSync and SyncScope.
	existing, _ := s.configStore.Load()
	files := s.effectiveFiles()

	if err := s.keychain.SetRemoteSecret(backend, secret); err != nil {
		return nil, fmt.Errorf("store %s credential: %w", backend, err)
	}
	snap, err := s.newRemoteBackend(backend, remote, secret, files)
	if err != nil {
		return nil, fmt.Errorf("build %s backend: %w", backend, err)
	}
	defer closeBackend(snap)
	if err := snap.Probe(); err != nil {
		return nil, fmt.Errorf("probe %s: %w", backend, err)
	}

	// Fresh staging for the fetch below.
	if err := os.RemoveAll(s.repoPath); err != nil {
		return nil, fmt.Errorf("clean staging: %w", err)
	}
	if err := os.MkdirAll(s.repoPath, 0755); err != nil {
		return nil, fmt.Errorf("create staging: %w", err)
	}

	manifest, err := snap.Head()
	if err != nil {
		return nil, fmt.Errorf("head: %w", err)
	}

	// The fresh struct's zero value clears LastSyncedHash — a new backend
	// is a new three-way base.
	saveCfg := func() error {
		cfg := SyncConfig{
			Backend:   backend,
			Remote:    remote,
			Branch:    "main",
			AutoSync:  existing.AutoSync,
			SyncScope: existing.SyncScope,
		}
		if backend == "webdav" {
			// Transition dual-write: older frontend builds and tests read
			// the flat fields; removed once the frontend uses Remote.
			cfg.WebDAVServer = remote.Server
			cfg.WebDAVPath = remote.Path
			cfg.WebDAVUser = remote.User
		}
		return s.configStore.Save(cfg)
	}

	if manifest != nil {
		// Remote already has a snapshot: verify the master password
		// against its ciphertext before anything is stored or decrypted.
		if err := snap.FetchAll(s.repoPath); err != nil {
			return nil, fmt.Errorf("fetch: %w", err)
		}
		salt, err := ReadSaltFile(s.repoPath)
		if err != nil {
			return nil, err
		}
		if salt == nil {
			return nil, utils.UserErr("salt_missing")
		}
		encKey := DeriveKey(masterPassword, salt)
		if err := verifyDecryptionScoped(files, s.repoPath, encKey); err != nil {
			return nil, utils.UserErr("master_password_mismatch")
		}
		if err := s.keychain.StoreEncryptionKey(encKey); err != nil {
			return nil, fmt.Errorf("store encryption key: %w", err)
		}

		tmpDir, err := os.MkdirTemp("", "remote-compare-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmpDir)
		if err := DecryptConfigFilesScoped(files, s.repoPath, tmpDir, encKey); err != nil {
			return nil, fmt.Errorf("decrypt remote for comparison: %w", err)
		}

		if err := saveCfg(); err != nil {
			return nil, err
		}

		if isConfigDirEmpty(files, s.dataDir) {
			// Local empty → adopt the remote wholesale.
			if _, err := s.pullSnapshot(files, s.repoPath, encKey, aggregateHash(manifest.FileHashes)); err != nil {
				return nil, err
			}
			return &SyncResult{Direction: SyncPull, Message: "仓库配置成功，已从远端同步配置"}, nil
		}
		same, err := storesMatchDir(s.stores, files, tmpDir)
		if err != nil {
			return nil, fmt.Errorf("compare configs: %w", err)
		}
		if !same {
			s.updateLastSyncResult("conflict", "")
			return &SyncResult{
				Direction: SyncConflict,
				Message:   "本地和远端配置不一致，请选择覆盖方向",
				Conflict: &ConflictInfo{
					LocalTime:  getConfigModTime(s.dataDir),
					RemoteTime: manifest.UpdatedAt,
				},
			}, nil
		}
		// Content identical: record the agreed hash as the new base.
		_, agg, err := hashStoresScoped(s.stores, files)
		if err != nil {
			return nil, err
		}
		s.setLastSyncedHash(agg)
		s.updateLastSyncResult("success", "")
		return &SyncResult{Message: "仓库配置成功"}, nil
	}

	// Fresh remote: generate salt, derive key, publish local config.
	salt, err := GenerateSalt()
	if err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	encKey := DeriveKey(masterPassword, salt)
	if err := s.keychain.StoreEncryptionKey(encKey); err != nil {
		return nil, fmt.Errorf("store encryption key: %w", err)
	}
	if err := saveCfg(); err != nil {
		return nil, err
	}
	// Seed the staging dir with the new salt and pass it as the fetched
	// dir: pushSnapshot reads the fetchedDir salt BEFORE its staging
	// rebuild, so the published remote salt matches the key derivation.
	// Without this, bootstrap would publish a second random salt and
	// re-attaching with the same master password would fail.
	if err := WriteSaltFile(s.repoPath, salt); err != nil {
		return nil, fmt.Errorf("write salt: %w", err)
	}
	if _, err := s.pushSnapshot(snap, files, encKey, s.repoPath, nil); err != nil {
		return nil, err
	}
	return &SyncResult{Direction: SyncPush, Message: "仓库配置成功"}, nil
}

// UpdateRemotePassword swaps the snapshot backend credential without
// touching any other configuration. Mirrors the git credential-edit
// semantics: an empty secret means "keep the stored one" — it is verified
// against the server but never rewritten. A non-empty secret is probed
// against the server first, and when the remote already holds a snapshot
// the master password must decrypt it — a typo can neither lock the device
// out of its own snapshot nor silently save a broken credential. Changing
// server/path/user is intentionally not possible here: that is a different
// sync target, which must go through delete + re-configure.
func (s *SyncService) UpdateRemotePassword(secret, masterPassword string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.requireUnlocked(); err != nil {
		return err
	}
	config, err := s.configStore.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if config.Backend == "" || config.Backend == "git" || !snapshotConfigured(config) {
		return fmt.Errorf("snapshot sync not configured")
	}
	if masterPassword == "" {
		return fmt.Errorf("master password is required")
	}

	// Empty secret = keep the stored credential (git-token semantics).
	store := secret != ""
	if !store {
		secret, _ = s.keychain.GetRemoteSecret(config.Backend)
		if secret == "" {
			return fmt.Errorf("%s credential not configured", config.Backend)
		}
	}

	snap, err := s.newRemoteBackend(config.Backend, config.Remote, secret, nil)
	if err != nil {
		return fmt.Errorf("build %s backend: %w", config.Backend, err)
	}
	defer closeBackend(snap)
	if err := snap.Probe(); err != nil {
		return fmt.Errorf("probe %s: %w", config.Backend, err)
	}

	manifest, err := snap.Head()
	if err != nil {
		return fmt.Errorf("head: %w", err)
	}
	if manifest != nil {
		if err := os.RemoveAll(s.repoPath); err != nil {
			return fmt.Errorf("clean staging: %w", err)
		}
		if err := os.MkdirAll(s.repoPath, 0755); err != nil {
			return fmt.Errorf("create staging: %w", err)
		}
		if err := snap.FetchAll(s.repoPath); err != nil {
			return fmt.Errorf("fetch: %w", err)
		}
		salt, err := ReadSaltFile(s.repoPath)
		if err != nil {
			return err
		}
		if salt == nil {
			return utils.UserErr("salt_missing")
		}
		encKey := DeriveKey(masterPassword, salt)
		if err := verifyDecryptionScoped(s.effectiveFiles(), s.repoPath, encKey); err != nil {
			return utils.UserErr("master_password_mismatch")
		}
	}

	if store {
		if err := s.keychain.SetRemoteSecret(config.Backend, secret); err != nil {
			return fmt.Errorf("store %s credential: %w", config.Backend, err)
		}
	}
	return nil
}
