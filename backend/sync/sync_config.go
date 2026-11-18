package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// defaultSyncedFiles is the default sync scope: the config files a device
// syncs before the user picks a scope in the sync settings. The store
// registry (SyncableStore, assembled in app.go), the git commit whitelist
// (git.go), the empty-dir probe
// (isConfigDirEmpty) and the password-rotation file list (ChangePassword)
// all iterate this slice so their scopes can never drift apart again.
//
// It is also the blob-name vocabulary: each syncable store's BlobName is
// one of these, and the sync layer moves a store's canonical plaintext in
// and out without knowing what any file contains.
//
// ai-sessions.json is intentionally absent — it is local-only data and must
// never be committed to the sync repo (a stray ai-sessions.json left in the
// repo dir from an old build stays untracked). skills.json is absent from
// the default set but offered opt-in in the scope picker (see
// syncableFiles).
//
// settings.json is intentionally absent from the default set: it is
// device-local (theme, paths, shells, keybindings, UI state) and changes too
// often to sync well. The syncable AI slice (model catalog + maxTurns) lives
// in ai.json instead; see AIConfigStore. A user who wants SFTP bookmarks or
// custom terminal themes on another machine can tick it in the scope picker.
//
// skills.json carries the skill preference layer (enabled/locked/sortOrder/
// origin) bundled with the full skill directories. It is opt-in like
// settings.json: skills can be large and multi-file, so a user who does not
// need them cross-device keeps them out of the sync payload (issue #1180).
// commands.json (commands/*.md + preferences) and mcp_clients.json (external
// tool servers, headers/env included) are opt-in for the same reason.
var defaultSyncedFiles = []string{
	"connections.json",
	"favorites.json",
	"ai.json",
	"quickCommands.json",
	"notes.json",
	"tunnels.json",
	"identities.json",
	"proxies.json",
}

// syncableFiles is the full menu offered by the sync-scope picker: the
// always-on legacy set plus the four opt-in entries (settings.json,
// skills.json, commands.json, mcp_clients.json).
var syncableFiles = append(append([]string{}, defaultSyncedFiles...),
	"settings.json", "skills.json", "commands.json", "mcp_clients.json")

// EffectiveSyncFiles resolves the user's scope against syncableFiles.
// A nil/empty scope returns the default set. Unknown names are dropped so a
// stale scope list from an older or newer build can never smuggle an
// unintended file into the repo.
func EffectiveSyncFiles(scope []string) []string {
	if len(scope) == 0 {
		return append([]string{}, defaultSyncedFiles...)
	}
	valid := make(map[string]bool, len(syncableFiles))
	for _, name := range syncableFiles {
		valid[name] = true
	}
	out := make([]string, 0, len(scope))
	for _, name := range scope {
		if valid[name] {
			delete(valid, name)
			out = append(out, name)
		}
	}
	return out
}

const syncConfigFileName = "sync-config.json"

// RemoteConfig is the connection description shared by every snapshot
// backend (webdav/sftp/s3/smb). The pattern mirrors the original WebDAV
// config: server + remote path + user, with the password-class secret kept
// out of the JSON in the keychain (Keychain.GetRemoteSecret) — only the
// non-secret fields live here.
type RemoteConfig struct {
	// Server is the WebDAV server URL; for sftp/smb host[:port]; for s3 the
	// endpoint (required — custom endpoints cover MinIO/OSS/COS, and AWS is
	// reachable through s3.amazonaws.com).
	Server string `json:"server,omitempty"`
	// Path is the remote directory (webdav/sftp/smb) or object key prefix
	// (s3) holding the snapshot.
	Path string `json:"path,omitempty"`
	// User authenticates the share; for s3 it is the access key (the secret
	// key rides in the keychain, matching the session-layer field semantics).
	User string `json:"user,omitempty"`
	// Bucket is s3-only.
	Bucket string `json:"bucket,omitempty"`
	// Region is s3-only (signature scope).
	Region string `json:"region,omitempty"`
	// URLStyle is s3-only: ""/"virtual" uses virtual-hosted URLs — required
	// by OSS/COS/OBS which reject path-style requests; "path" keeps
	// endpoint/bucket/key addressing for AWS S3 and MinIO.
	URLStyle string `json:"urlStyle,omitempty"`
	// Domain is smb-only (NTLM domain; empty = local account).
	Domain string `json:"domain,omitempty"`
	// Share is smb-only: the share name mounted before any file operation.
	Share string `json:"share,omitempty"`
}

type SyncConfig struct {
	RepoURL  string `json:"repoUrl"`
	Branch   string `json:"branch"`
	Username string `json:"username"`
	AutoSync bool   `json:"autoSync"`
	// SyncScope is this device's per-file sync selection over syncableFiles.
	// nil/empty resolves to the default set (defaultSyncedFiles).
	SyncScope []string `json:"syncScope,omitempty"`
	// Backend selects the sync transport: "git" (default, private git repo)
	// or one of the snapshot backends "webdav"/"sftp"/"s3"/"smb".
	Backend string       `json:"backend,omitempty"`
	Remote  RemoteConfig `json:"remote,omitempty"`
	// Legacy flat WebDAV fields — read by Load for configs written before
	// RemoteConfig existed, written back only as a transition dual-write in
	// ConfigureRepoRemote. New code must use Remote.
	WebDAVServer string `json:"webdavServer,omitempty"`
	WebDAVPath   string `json:"webdavPath,omitempty"`
	WebDAVUser   string `json:"webdavUser,omitempty"`
	// LastSyncedHash is the aggregate content hash both sides agreed on at
	// the last successful snapshot sync (three-way base for snapshot mode).
	LastSyncedHash string `json:"lastSyncedHash,omitempty"`

	LastSyncAt     time.Time `json:"lastSyncAt"`
	LastSyncStatus string    `json:"lastSyncStatus"`
	LastSyncError  string    `json:"lastSyncError"`
}

type SyncConfigStore struct {
	configDir string
}

func NewSyncConfigStore(configDir string) *SyncConfigStore {
	return &SyncConfigStore{configDir: configDir}
}

func (s *SyncConfigStore) filePath() string {
	return filepath.Join(s.configDir, syncConfigFileName)
}

func (s *SyncConfigStore) Save(config SyncConfig) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.filePath(), data, 0600)
}

func (s *SyncConfigStore) Load() (SyncConfig, error) {
	data, err := os.ReadFile(s.filePath())
	if err != nil {
		if os.IsNotExist(err) {
			return SyncConfig{Branch: "main"}, nil
		}
		return SyncConfig{}, err
	}
	var config SyncConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return SyncConfig{Branch: "main"}, nil
	}
	if config.Branch == "" {
		config.Branch = "main"
	}
	// One-time lift of the pre-RemoteConfig flat WebDAV fields: older
	// configs carry the connection in webdavServer/webdavPath/webdavUser.
	// Remote wins when both are present (a config re-saved by a new build).
	if config.Remote.Server == "" && config.WebDAVServer != "" {
		config.Remote = RemoteConfig{
			Server: config.WebDAVServer,
			Path:   config.WebDAVPath,
			User:   config.WebDAVUser,
		}
	}
	return config, nil
}
