package sync

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var ErrWrongSyncPassword = errors.New("WRONG_SYNC_PASSWORD")

type SyncService struct {
	configDir   string
	repoPath    string
	keychain    *Keychain
	configStore *SyncConfigStore
	mu          sync.Mutex
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

func NewSyncService() (*SyncService, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	appDir := filepath.Join(configDir, "uniTerm")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		return nil, err
	}

	return &SyncService{
		configDir:   appDir,
		repoPath:    filepath.Join(appDir, "sync-repo"),
		keychain:    NewKeychain(),
		configStore: NewSyncConfigStore(appDir),
	}, nil
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

// Sync runs a full sync cycle: clone/open → encrypt → commit → fetch → compare → push/pull → decrypt.
func (s *SyncService) Sync() (*SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	config, err := s.configStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
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

	// 1. Clone or open repo
	repo, err := CloneOrOpen(s.repoPath, config.RepoURL, config.Branch, username, token)
	if err != nil {
		s.updateLastSyncResult("failed", fmt.Sprintf("open repo: %v", err))
		return nil, fmt.Errorf("open repo: %w", err)
	}

	// 2. Encrypt and commit only if local config has actually changed.
	// A decrypt error means the stored master password / key cannot
	// open the remote ciphertext (wrong password or corrupted blob);
	// refuse to push and surface the error to the user rather than
	// overwriting the only good remote copy with locally-derived
	// ciphertext (SYNC-P1-11).
	committed := false
	same, cmpErr := s.compareLocalWithRepo(encKey)
	if cmpErr != nil {
		s.updateLastSyncResult("password_mismatch", cmpErr.Error())
		return nil, cmpErr
	}
	if !same {
		if err := EncryptConfigFiles(s.configDir, s.repoPath, encKey, s.keychain); err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("encrypt files: %v", err))
			return nil, fmt.Errorf("encrypt files: %w", err)
		}

		committed, err = repo.StageAndCommit(commitMsg("uniTerm config sync"))
		if err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("commit: %v", err))
			return nil, fmt.Errorf("commit: %w", err)
		}
	}

	// 4. Fetch
	if err := repo.Fetch(username, token); err != nil {
		if committed {
			if pushErr := repo.Push(username, token); pushErr != nil {
				s.updateLastSyncResult("failed", fmt.Sprintf("push: %v", pushErr))
				return nil, fmt.Errorf("push to empty remote: %w", pushErr)
			}
			s.updateLastSyncResult("success", "")
			return &SyncResult{Direction: SyncPush, Message: "配置已上传"}, nil
		}
		s.updateLastSyncResult("success", "")
		return &SyncResult{Message: "已是最新"}, nil
	}

	// 5. Compare heads
	direction, localTime, remoteTime, err := repo.CompareHeads(config.Branch)
	if err != nil {
		s.updateLastSyncResult("failed", fmt.Sprintf("compare: %v", err))
		return nil, fmt.Errorf("compare: %w", err)
	}

	switch direction {
	case SyncNone:
		s.updateLastSyncResult("success", "")
		return &SyncResult{Message: "已是最新"}, nil

	case SyncPush:
		if err := repo.Push(username, token); err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("push: %v", err))
			return nil, fmt.Errorf("push: %w", err)
		}
		s.updateLastSyncResult("success", "")
		return &SyncResult{Direction: SyncPush, Message: "配置已上传"}, nil

	case SyncPull:
		if err := repo.Pull(username, token); err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("pull: %v", err))
			return nil, fmt.Errorf("pull: %w", err)
		}
		if err := DecryptConfigFiles(s.repoPath, s.configDir, encKey, s.keychain); err != nil {
			s.updateLastSyncResult("failed", fmt.Sprintf("decrypt files: %v", err))
			return nil, fmt.Errorf("decrypt files: %w", err)
		}
		s.updateLastSyncResult("success", "")
		return &SyncResult{Direction: SyncPull, Message: "配置已下载"}, nil

	case SyncConflict:
		if localTime == nil {
			t := time.Time{}
			localTime = &t
		}
		if remoteTime == nil {
			t := time.Time{}
			remoteTime = &t
		}
		return &SyncResult{
			Direction: SyncConflict,
			Conflict: &ConflictInfo{
				LocalTime:  *localTime,
				RemoteTime: *remoteTime,
			},
		}, nil
	}

	return &SyncResult{Message: "已是最新"}, nil
}

// ResolveConflict handles a conflict by forcing push or reset.
func (s *SyncService) ResolveConflict(useLocal bool) (*SyncResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	config, err := s.configStore.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	username := config.Username
	token := s.getToken()

	repo, err := CloneOrOpen(s.repoPath, config.RepoURL, config.Branch, username, token)
	if err != nil {
		return nil, fmt.Errorf("open repo: %w", err)
	}

	encKey, err := s.keychain.GetEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}

	if useLocal {
		if err := EncryptConfigFiles(s.configDir, s.repoPath, encKey, s.keychain); err != nil {
			return nil, fmt.Errorf("encrypt files: %w", err)
		}
		if _, err := repo.StageAndCommit(commitMsg("uniTerm config sync (resolve conflict)")); err != nil {
			return nil, fmt.Errorf("commit: %w", err)
		}
		if err := repo.ForcePush(username, token); err != nil {
			return nil, fmt.Errorf("force push: %w", err)
		}
		s.updateLastSyncResult("success", "")
		return &SyncResult{Direction: SyncPush, Message: "已用本地配置覆盖远端"}, nil
	}

	if err := repo.ResetToRemote(config.Branch); err != nil {
		return nil, fmt.Errorf("reset to remote: %w", err)
	}

	if err := DecryptConfigFiles(s.repoPath, s.configDir, encKey, s.keychain); err != nil {
		return nil, fmt.Errorf("decrypt files: %w", err)
	}

	s.updateLastSyncResult("success", "")
	return &SyncResult{Direction: SyncPull, Message: "已用远端配置覆盖本地"}, nil
}

// TestConnection verifies the repo is reachable with stored credentials.
func (s *SyncService) TestConnection() error {
	config, err := s.configStore.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if config.RepoURL == "" {
		return fmt.Errorf("仓库地址未设置")
	}
	username := config.Username
	token := s.getToken()
	return TestConnection(config.RepoURL, username, token)
}

func (s *SyncService) updateLastSyncResult(status string, errMsg string) {
	config, _ := s.configStore.Load()
	config.LastSyncAt = time.Now()
	config.LastSyncStatus = status
	config.LastSyncError = errMsg
	_ = s.configStore.Save(config)
}

// IsAutoSyncEnabled returns whether auto sync is enabled and configured.
func (s *SyncService) IsAutoSyncEnabled() bool {
	config, _ := s.configStore.Load()
	return config.AutoSync && config.RepoURL != ""
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

	repo, err := CloneOrOpen(s.repoPath, repoURL, "main", username, token)
	if err != nil {
		return nil, fmt.Errorf("clone/open repo: %w", err)
	}

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
			return nil, fmt.Errorf("主密码错误，无法解密远端配置")
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

		if err := DecryptConfigFiles(s.repoPath, tmpDir, encKey, nil); err != nil {
			return nil, fmt.Errorf("decrypt remote for comparison: %w", err)
		}

		localEmpty := isConfigDirEmpty(s.configDir)
		remoteEmpty := isConfigDirEmpty(tmpDir)

		if localEmpty {
			// Local has no config — pull remote to local
			if err := DecryptConfigFiles(s.repoPath, s.configDir, encKey, s.keychain); err != nil {
				return nil, fmt.Errorf("decrypt files: %w", err)
			}
			cfg := SyncConfig{
				RepoURL:  repoURL,
				Branch:   "main",
				Username: username,
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
			same, err := compareConfigDirs(s.configDir, tmpDir, s.keychain)
			if err != nil {
				return nil, fmt.Errorf("compare configs: %w", err)
			}
			if !same {
				cfg := SyncConfig{
					RepoURL:  repoURL,
					Branch:   "main",
					Username: username,
				}
				if token != "" {
					_ = s.keychain.SetGitToken(token)
				}
				_ = s.configStore.Save(cfg)
				localTime := getConfigModTime(s.configDir)
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
			cfg := SyncConfig{
				RepoURL:  repoURL,
				Branch:   "main",
				Username: username,
			}
			if token != "" {
				_ = s.keychain.SetGitToken(token)
			}
			_ = s.configStore.Save(cfg)
			if err := DecryptConfigFiles(s.repoPath, s.configDir, encKey, s.keychain); err != nil {
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
	if err := EncryptConfigFiles(s.configDir, s.repoPath, encKey, s.keychain); err != nil {
		return nil, fmt.Errorf("encrypt files: %w", err)
	}

	if _, err := repo.StageAndCommit(commitMsg("uniTerm config sync")); err != nil {
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

	cfg := SyncConfig{
		RepoURL:  repoURL,
		Branch:   "main",
		Username: username,
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
	if err := DecryptConfigFiles(s.repoPath, s.configDir, encKey, s.keychain); err != nil {
		return nil, fmt.Errorf("decrypt files: %w", err)
	}

	s.updateLastSyncResult("success", "")
	return &SyncResult{Direction: SyncPush, Message: "仓库配置成功"}, nil
}

// getConfigModTime returns the latest modification time of config files in a directory.
func getConfigModTime(dir string) time.Time {
	var latest time.Time
	for _, name := range []string{"connections.json", "settings.json", "quickCommands.json"} {
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

// isConfigDirEmpty returns true if the config dir has no meaningful data.
// Counts every persisted JSON the app cares about — not only connections —
// so a user with settings / AI / quick-commands but no connections is not
// treated as "empty" and silently overwritten on first sync
// (SYNC-P0-1).
func isConfigDirEmpty(dir string) bool {
	for _, name := range []string{"connections.json", "settings.json", "quickCommands.json", "ai-sessions.json", "skills.json"} {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if len(data) == 0 {
			continue
		}
		// Treat as non-empty if the file parses to anything other than
		// an explicitly empty wrapper.
		var probe map[string]json.RawMessage
		if err := json.Unmarshal(data, &probe); err != nil {
			// Unparseable — still treat as non-empty so the user's data
			// is never silently nuked.
			return false
		}
		if len(probe) > 0 {
			return false
		}
	}
	return true
}

// compareConfigDirs compares two decrypted config directories.
// localDir is the local config directory; remoteDir is the decrypted remote copy.
// Passwords are backfilled from keychain on the local side before comparison.
func compareConfigDirs(localDir, remoteDir string, kc *Keychain) (bool, error) {
	for _, name := range []string{"connections.json", "settings.json", "quickCommands.json"} {
		same, err := compareConfigFiles(filepath.Join(localDir, name), filepath.Join(remoteDir, name), kc)
		if err != nil {
			return false, err
		}
		if !same {
			return false, nil
		}
	}
	return true, nil
}

// compareConfigFiles compares two config files after backfilling local passwords from keychain.
// JSON is re-marshaled via json.MarshalIndent so Go's encoding/json sorts map
// keys deterministically before byte comparison — prevents spurious diffs from
// non-deterministic key ordering in the underlying JSON.
func compareConfigFiles(localPath, remotePath string, kc *Keychain) (bool, error) {
	localData, err := os.ReadFile(localPath)
	if err != nil {
		localData = []byte("{}")
	}
	remoteData, err := os.ReadFile(remotePath)
	if err != nil {
		remoteData = []byte("{}")
	}

	var localObj, remoteObj map[string]interface{}
	if err := json.Unmarshal(localData, &localObj); err != nil {
		return false, fmt.Errorf("parse local %s: %w", localPath, err)
	}
	if err := json.Unmarshal(remoteData, &remoteObj); err != nil {
		return false, fmt.Errorf("parse remote %s: %w", remotePath, err)
	}

	// Backfill passwords from keychain on the local side so both sides are comparable
	backfillFromKeychain(localObj, kc)

	localNorm, err := json.MarshalIndent(localObj, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal local: %w", err)
	}
	remoteNorm, err := json.MarshalIndent(remoteObj, "", "  ")
	if err != nil {
		return false, fmt.Errorf("marshal remote: %w", err)
	}
	return string(localNorm) == string(remoteNorm), nil
}

func backfillFromKeychain(obj map[string]interface{}, kc *Keychain) {
	if kc == nil {
		return
	}
	// Backfill connection passwords
	if conns, ok := obj["connections"].([]interface{}); ok {
		for _, c := range conns {
			if cm, ok := c.(map[string]interface{}); ok {
				if cm["authType"] != "password" {
					continue
				}
				pw, _ := cm["password"].(string)
				if pw == "" {
					if id, ok := cm["id"].(string); ok {
						if kcPw, err := kc.GetPassword(id); err == nil && kcPw != "" {
							cm["password"] = kcPw
						}
					}
				}
			}
		}
	}
	// Backfill model apiKeys from keychain (settings.json: ai.models[].apiKey)
	if ai, ok := obj["ai"].(map[string]interface{}); ok {
		if models, ok := ai["models"].([]interface{}); ok {
			for _, m := range models {
				if mm, ok := m.(map[string]interface{}); ok {
					ak, _ := mm["apiKey"].(string)
					if ak == "" {
						if id, ok := mm["id"].(string); ok {
							if kcAk, err := kc.GetModelAPIKey(id); err == nil && kcAk != "" {
								mm["apiKey"] = kcAk
							}
						}
					}
				}
			}
		}
	}
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
		return fmt.Errorf("远端仓库数据异常，缺少密钥盐值")
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

	encrypted, err := repo.ReadRemoteFile(config.Branch, "connections.json")
	if err != nil {
		encrypted, err = os.ReadFile(filepath.Join(s.repoPath, "connections.json"))
		if err != nil {
			return ErrWrongSyncPassword
		}
	}

	if _, err := decryptBytes(string(encrypted), key); err != nil {
		return ErrWrongSyncPassword
	}
	return nil
}

// ChangePassword re-encrypts all synced files with a new master password
// and a fresh random salt. A new salt is required so PBKDF2 work cannot be
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
	if config.RepoURL == "" {
		return fmt.Errorf("no repo configured")
	}

	oldSalt, err := ReadSaltFile(s.repoPath)
	if err != nil {
		return fmt.Errorf("read salt: %w", err)
	}
	if oldSalt == nil {
		return fmt.Errorf("远端仓库数据异常，缺少密钥盐值")
	}

	// Verify old password can decrypt the repo.
	oldKey := DeriveKey(oldPassword, oldSalt)
	if err := verifyDecryption(s.repoPath, oldKey); err != nil {
		return fmt.Errorf("当前密码错误")
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

	// Re-encrypt every existing repo ciphertext: decrypt with oldKey,
	// re-encrypt with newKey, atomic rename. A crash before the rename
	// leaves the original ciphertext intact and decryptable with oldKey.
	repoFiles := []string{"connections.json", "settings.json", "quickCommands.json"}
	for _, name := range repoFiles {
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

	if _, err := repo.StageAndCommit(commitMsg("uniTerm config sync (change password)")); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	if err := repo.Push(username, token); err != nil {
		return fmt.Errorf("push: %w", err)
	}

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

	return s.configStore.Save(SyncConfig{Branch: "main"})
}

// commitMsg builds a commit message. Hostname and user identity are
// intentionally NOT included — the sync repo is often shared across
// devices and machines, and leaking os.Hostname() to a public-by-mistake
// repo is a privacy regression (SYNC-P1-4).
func commitMsg(action string) string {
	return fmt.Sprintf("%s | %s", action, time.Now().Format(time.RFC3339))
}

// compareLocalWithRepo decrypts repo files and compares them with local config.
// Returns true if the local config content matches what's already in the repo.
// A decrypt error is surfaced (not swallowed) so callers can refuse to push
// over an undecryptable remote (SYNC-P1-11).
func (s *SyncService) compareLocalWithRepo(encKey []byte) (bool, error) {
	if !repoHasFiles(s.repoPath) {
		return false, nil
	}
	tmpDir, err := os.MkdirTemp("", "sync-cmp")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmpDir)

	if err := DecryptConfigFiles(s.repoPath, tmpDir, encKey, nil); err != nil {
		return false, fmt.Errorf("decrypt remote: %w", err)
	}
	return compareConfigDirs(s.configDir, tmpDir, s.keychain)
}

// repoHasFiles returns true if the repo directory contains encrypted config files.
func repoHasFiles(repoPath string) bool {
	for _, name := range []string{"connections.json", "settings.json", "quickCommands.json"} {
		if _, err := os.Stat(filepath.Join(repoPath, name)); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// verifyDecryption checks that the given key can decrypt the remote config files.
func verifyDecryption(repoPath string, key []byte) error {
	connPath := filepath.Join(repoPath, "connections.json")
	if _, err := os.Stat(connPath); os.IsNotExist(err) {
		return nil
	}
	tmpDir, err := os.MkdirTemp("", "sync-verify")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	return DecryptConfigFiles(repoPath, tmpDir, key, nil)
}
