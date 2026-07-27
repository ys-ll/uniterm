# Phase 1 Audit — backend/sync/

**Auditor:** gsd-code-reviewer (Phase 1)
**Date:** 2026-07-28
**Scope:** `backend/sync/*.go` (`sync_service.go`, `sync_config.go`, `crypto.go`, `git.go`, `keychain.go`)

---

## Findings (by severity)

### P0 — Critical

**SYNC-P0-1: `Sync()` local-empty-overwrite path overwrites ALL three config files even when only `connections.json` is empty.**
- File: `backend/sync/sync_service.go:444-457` (`isConfigDirEmpty`), invoked at `backend/sync/sync_service.go:304-305` and used to gate the destructive "local empty" branch at `sync_service.go:307-323`.
- Scenario: User has rich `settings.json` (custom AI prompts, theme, etc.) and many `quickCommands.json` entries but zero `connections.json` entries (e.g. they only use SFTP/FTP which are stored separately, or they just connected to a fresh account). They sync to an existing remote repo (encrypted with same master password). `isConfigDirEmpty` only checks `connections.json` — returns `true`. Branch at line 307 fires, `DecryptConfigFiles(s.repoPath, s.configDir, ...)` overwrites `settings.json` and `quickCommands.json` with the remote versions. All local customizations vanish silently.
- Failure: Silent data loss of local AI/quick-command configuration; user only notices when settings they expected are gone.
- Fix category: Multi-file emptiness check, or drop the "local empty" branch entirely and route through `compareConfigDirs` like the normal Sync() path does.

**SYNC-P0-2: `ResolveConflict()` has no mutex — races with `Sync()`.**
- File: `backend/sync/sync_service.go:183-225`. Note other public mutating methods (`Sync` line 74, `ConfigureRepo` line 267, `ChangePassword` line 602, `DeleteRepo` line 659) all take `s.mu.Lock()`. `ResolveConflict` does not.
- Scenario: User clicks "Sync" (takes `s.mu` and enters a multi-second Push/Pull flow). Before it returns, the conflict-detection UI prompts the user to resolve; the user clicks "Use remote" which calls `ResolveConflict(useLocal=false)` from a separate goroutine/tick. Both paths manipulate `g.repo` (worktree) on disk — `repo.ResetToRemote` (line 215) from one path racing with the `StageAndCommit`/push from another. go-git is not safe for concurrent worktree mutations on the same `*git.Repository`. Result: index lock file collision (`.git/index.lock`), or worse, a half-reset worktree being pushed.
- Failure: Worktree corruption, `.git/index.lock` left behind, or silent loss of in-flight commits.
- Fix category: Add `s.mu.Lock(); defer s.mu.Unlock()` at the top of `ResolveConflict`, or refactor both to share a finer-grained per-repo lock.

### P1 — High

**SYNC-P1-1: AES-GCM `Seal`/`Open` is called with `additionalData = nil` — file identity is not authenticated.**
- File: `backend/sync/crypto.go:276` (`aesGCM.Seal(nonce, nonce, plaintext, nil)`) and `backend/sync/crypto.go:298` (`aesGCM.Open(nil, nonce, ciphertext, nil)`).
- Scenario: An attacker who can write to the sync repo (e.g. through a leaked token with write access, or a malicious upstream) cannot decrypt the ciphertext (no key) but *can* swap ciphertexts between files: replace `settings.json` ciphertext with `connections.json` ciphertext. On the next sync, the user decrypts the swapped blob and writes the wrong file to disk. Worse: a malleability attack against AES-GCM is amplified by the missing AAD — flipping a ciphertext bit in one file yields bit flips at the same offset in another when the swap target shares the same key.
- Failure: Cross-file ciphertext swap; integrity guarantee is per-blob, not per-file.
- Fix category: Bind `additionalData` to a stable file identifier (e.g. the filename) and validate on decrypt.

**SYNC-P1-2: `compareConfigFiles` uses non-deterministic `json.Marshal` ordering on `map[string]interface{}` — produces false-positive "different" results.**
- File: `backend/sync/sync_service.go:476-496`, specifically `localNorm, _ := json.Marshal(localObj)` at line 493 and `remoteNorm, _ := json.Marshal(remoteObj)` at line 494.
- Scenario: `localObj` and `remoteObj` are `map[string]interface{}` parsed from JSON; Go map iteration order is randomized. `json.Marshal` of a map emits keys in sorted order, so this should be deterministic for identical content — *however*, this depends on the keys being identical sets. If local has an extra key (e.g. user added a UI field) that's missing in remote (different code version, settings schema migration), the JSON serializations differ in length and key set, breaking equality. More subtle: `[]interface{}` orderings inside e.g. `connections` are *preserved*, so reordering by the user is detected as "different" only if the same connection's internal map differs (no problem) but reordering the connections list itself breaks equality (because insertion order in `[]interface{}` is preserved). However, the deeper problem is that the *normalization* is `json.Marshal` without indentation — but the encryption step writes with `json.MarshalIndent` (crypto.go:78, 113). So the bytes compared in `compareLocalWithRepo` are MINIFIED, but the on-disk source may be INDENTED. Indentation differences in `connections.json` shouldn't matter because they go through `json.Unmarshal` first. Still — the comparison can't detect when remote is encrypted with stale keys that match semantic content (since both sides are decrypted raw JSON maps, this part is OK). The actual subtle bug: when `kc != nil` and `backfillFromKeychain` modifies *local* but not *remote*, key order is unaffected but the passwords differ.
- Net failure: spurious "different" → spurious sync commit. CONCERNS already flags this; we re-flag that the fix must compare *semantic* equality (sorted keys + normalized types), not naive `json.Marshal`.
- Fix category: Implement `jsonEqual(a, b interface{}) bool` that walks recursively comparing sorted-keys maps with type normalization.

**SYNC-P1-3: `getConfigModTime` uses local filesystem stat times; `CompareHeads` uses git committer timestamps. Conflict UI reports times from two unrelated sources.**
- File: `backend/sync/sync_service.go:429-441` (`getConfigModTime`) called at `backend/sync/sync_service.go:340-341` inside the conflict response — vs. `backend/sync/git.go:201-202` which returns `committer.When` from the git objects.
- Scenario: User sees a "conflict" UI with `LocalTime: <mtime of connections.json on disk>` and `RemoteTime: <committer time of HEAD on origin/main>`. These are different measurement systems: filesystem mtime is when the file was *last written by uniterm*, committer time is when the commit was made (could be years later if the repo was cloned stale). User can't make an informed choice between "local" and "remote" because the times answer different questions.
- Failure: Misleading conflict UX; users may pick the wrong side.
- Fix category: Use one source — either record the last-sync-time in `SyncConfig` and compare against it, or always use git committer time (which means *not* decrypting into tmpDir to get mtime).

**SYNC-P1-4: `commitMsg` leaks hostname into every commit.**
- File: `backend/sync/sync_service.go:672-676` (`commitMsg`).
- Scenario: Commit message format is `<action> | <hostname> | <rfc3339>`. Hostname (e.g. `janet-mbp.local`, `workstation-42.corp.example.com`) lands in the git repo's history — visible to anyone with repo read access. If the user syncs to a shared org repo, the commit log deanonymizes the user's machine names.
- Failure: Information disclosure into remote repo; hostname useful for fingerprinting in targeted attacks.
- Fix category: Use a stable per-install device ID stored in `SyncConfig` (UUID), not `os.Hostname()`; or hash the hostname; or accept an empty hostname.

**SYNC-P1-5: `Keychain.GetGitToken()` swallows keyring errors as empty token — Sync() proceeds with no auth.**
- File: `backend/sync/keychain.go:53-59` and `backend/sync/sync_service.go:67-70` (`getToken`).
- Scenario: User previously configured sync, keychain entry for `git-token` was wiped by an OS-level password reset (common on Linux after distro upgrade with new keyring backend, or after `keyring` daemon restart with a different default). `GetGitToken` returns `("", nil)` (not an error). `getToken` returns `""`. Sync() then calls `repo.Fetch(username, "")` with empty password. With an empty `Password`, go-git's `BasicAuth` sends `username:` as the base64 — GitHub returns 401 (or `Bad credentials`), but other providers may accept "any anonymous user" or may cache a prior `WWW-Authenticate` and retry. Worse: any internal go-git logic that compares "token == "" means unauthenticated" may silently skip auth when the user actually has credentials stored.
- Failure: Silent authentication fallback that misleads the user into thinking their token is invalid.
- Fix category: Return a distinct error from `GetGitToken` when the entry is missing, surface it to UI.

**SYNC-P1-6: `ChangePassword` re-uses the SAME salt to derive old and new keys.**
- File: `backend/sync/sync_service.go:621-628`, salt loaded at 613.
- Scenario: User changes their master password. Old key `k1 = PBKDF2(p1, salt, 600k, 32)` and new key `k2 = PBKDF2(p2, salt, 600k, 32)`. Because the salt is unchanged, an attacker who can see both old `k1` (via prior keychain leak OR via salt + offline pbkdf2 guess of p1) gains information about `k2` proportional to the prefix overlap of p1/p2. PBKDF2 with HMAC-SHA256 is not "memory-hard" against GPU brute force anyway, and re-using the same salt for two passwords lets an attacker amortize the PBKDF2 work across both. More importantly, if p1 happens to equal p2 (user "changes" to the same password, or types a typo and corrects), `k1 == k2` and the commit produces a new commit with the SAME ciphertexts — which the repo then hashes-with-salt-changed, providing no information to a defender. Subtle: this also defeats password rotation as a security control.
- Failure: Password rotation doesn't actually invalidate old-ciphertext knowledge in the same way it should.
- Fix category: Generate a fresh salt on every password change; persist it; accept that all historical ciphertexts become un-decryptable without re-encrypting them with old → new key escrow.

**SYNC-P1-7: Plaintext `sync-config.json` (repo URL, username, host fingerprint) lives in `os.UserConfigDir()/uniTerm/`.**
- File: `backend/sync/sync_config.go:12-20` (struct definition), `sync_config.go:34-39` (`Save` writes plaintext via `os.WriteFile` with mode 0600).
- Scenario: The `SyncConfig` struct contains `RepoURL` (which often embeds org/owner info like `git@github.com:acme-corp/prod-bastion`), `Username` (often an email), `AutoSync` flag, and `LastSyncError` (which CONCERNS already flagged may contain credential material). File mode 0600 mitigates other-user reads, but backup tooling (`Backup`, `Time Machine`, file-sync tools that include the config dir) capture the file as-is. CONCERNS does not explicitly call out sync-config.json as plaintext.
- Failure: Repo URL leakage in backups; `LastSyncError` may contain usernames or partial tokens.
- Fix category: Encrypt at rest using a machine-local key (DPAPI on Windows, Keychain on macOS, libsecret on Linux), or move `RepoURL`/`Username` into the keychain alongside `git-token`.

**SYNC-P1-8: `isConfigDirEmpty` parses JSON without size limit and trusts the unmarshaled wrapper.**
- File: `backend/sync/sync_service.go:444-457`.
- Scenario: A user-supplied `connections.json` (or a corrupted one) that is a giant file (say, a 2GB blob) triggers `os.ReadFile` + `json.Unmarshal` in this hot-path helper. `json.Unmarshal` on a 2GB file will balloon memory and stall the process. Adversarial: if sync repo is hosted on a less-trusted host (GitHub fork-as-pull-request scenario — repo comes from outside via OAuth app or shared URL), the decrypted repo could contain a maliciously large JSON file that triggers this on every `Sync()`.
- Failure: UI freeze / OOM on large/malicious JSON.
- Fix category: Check file size before `os.ReadFile` (e.g. 50MB hard cap); fail closed with a clear error.

**SYNC-P1-9: `StageAndCommit` uses `wt.Add(".")` — non-config files (`.DS_Store`, `.gitignore_*`, IDE temp files) accidentally staged.**
- File: `backend/sync/git.go:101` (`wt.Add(".")`).
- Scenario: After sync, the `s.repoPath` may contain stray files the user dropped there (a downloaded `data.csv`, a debug log). The next sync does `StageAndCommit(commitMsg(...))` which does `wt.Add(".")` and commits every untracked file to the user's sync repo. Encryption is applied only to known filenames (`connections.json`, `settings.json`, `quickCommands.json` in `EncryptConfigFiles`), so plaintext junk gets *committed unencrypted*.
- Failure: User drops a private SSH key in the sync repo → next sync commits it in plaintext.
- Fix category: Use `wt.AddWithOptions` with explicit file globs (`connections.json`, `settings.json`, `quickCommands.json`, `.sync-salt`) or maintain a `.gitignore` automatically.

**SYNC-P1-10: Network operations (`Fetch`, `Push`, `Pull`) have no retry, no timeout config, no exponential backoff — single transient failure (DNS blip, 503) aborts the whole sync.**
- File: `backend/sync/git.go:118-140` (Push/Fetch/Pull), called from `backend/sync/sync_service.go` flows.
- Scenario: Mid-sync on flaky hotel Wi-Fi, GitHub returns 503. The single attempt surfaces as a `failed` status; user has no auto-recovery path. `IsAutoSyncEnabled` returns true if `AutoSync` is checked but `ConfigureRepo` never implements a periodic caller (see SYNC-P3-1) — so the user must manually retry. Many enterprise Gitea/GitLab instances have rate-limit-on-error semantics that DO recover within seconds.
- Failure: Sync fails for transient conditions; users blame the app.
- Fix category: Wrap push/fetch/pull in retry-with-backoff (e.g. 3 retries: 2s, 5s, 15s); expose timeout config.

**SYNC-P1-11: Decryption errors during `Sync()` are swallowed as "treat as changed".**
- File: `backend/sync/sync_service.go:691` (`if err := DecryptConfigFiles(s.repoPath, tmpDir, encKey, nil); err != nil { return false, nil }`).
- Scenario: The remote repo's data is corrupted (broken master password, modified ciphertext, format change from a future migration). `compareLocalWithRepo` returns `(false, nil)` → `if !same` branch fires → `EncryptConfigFiles` is called, which **overwrites the local repo working copy** with freshly re-encrypted local data. The next `repo.Push()` uploads that as the new "latest version" — clobbering the only copy of the legitimate remote state.
- Failure: A single wrong-password push permanently destroys the remote sync repo.
- Fix category: Distinguish "decrypt-failed-bad-password" from "decrypt-failed-data-corrupt" and refuse to overwrite in both cases; require explicit user "yes, push over broken remote".

### P2 — Medium

**SYNC-P2-1: `sync` package is missing a `Trylock` or busy-state signal; rapid UI clicks queue up silently.**
- File: `backend/sync/sync_service.go:20` (mutex field), `sync_service.go:74` (lock site).
- Scenario: User double-clicks the Sync toolbar button. First call takes `s.mu` and runs (perhaps 10s during a Push). Second call blocks for 10s — when it finally runs, it sees a *post*-first-sync state and the visible spinner has long since stopped. The user perceives "Sync did nothing" for the second click.
- Failure: Poor UX, plus potential for `Sync` to process inconsistent mid-flight state.
- Fix category: Use `sync.Mutex` → a state machine with `Idle/Working/Error/Conflict` channels; return early on `Working` with a "Sync already in progress" message.

**SYNC-P2-2: `backfillFromKeychain` in sync_service.go only backfills `authType == "password"` entries — SSH key / certificate auth is NEVER encrypted in backups.**
- File: `backend/sync/sync_service.go:498-537`, specifically the `if cm["authType"] != "password" { continue }` at line 506.
- Scenario: User has SSH connections with private key references (`authType: "key", privateKey: "/Users/janet/.ssh/id_ed25519", keyPassphrase: "..."`). The connection JSON contains the key *path*, not the key content (in current schemas), but `keyPassphrase` IS a secret. Encrypt path passes through unchanged — passphrase stays in plaintext form if it was in the JSON, or is missing entirely if it lived only in the keychain.
- Failure: SSH key passphrases leak in plaintext; partial-coverage of "encryption" claims.
- Fix category: Iterate over all auth types and call `kc.GetKeyPassphrase(id)` regardless of type; document what is and isn't backed up.

**SYNC-P2-3: `compareConfigFiles` ignores `json.Unmarshal` errors — falls back to `{}` on parse failure.**
- File: `backend/sync/sync_service.go:477-488`.
- Scenario: One side of the comparison is a malformed JSON (partial sync, mid-write). Both bytes passed through `json.Unmarshal` with errors discarded → both `localObj`/`remoteObj` are nil → both marshal to `null` → comparison says "same" → `if !same` is false → no commit, even though one is needed.
- Failure: Inconsistent state silently unreported; can hide data corruption over time.
- Fix category: Surface unmarshal error to caller; abort Sync() with a clear "config file corrupted" error.

**SYNC-P2-4: `repoHasFiles` is a precondition *looseness* — any of the three files existing returns true.**
- File: `backend/sync/sync_service.go:696-703`.
- Scenario: Repo was mid-clone and contains only `quickCommands.json` (because `connections.json` failed to write). `repoHasFiles` returns `true` → next sync proceeds assuming fully-populated repo → comparison writes a blank `connections.json` to remote.
- Failure: Partial repo state triggers destructive fill.
- Fix category: Require ALL three files (or whatever known set) before claiming "repo populated"; fall back to `initEmpty` if any are missing.

**SYNC-P2-5: `DecryptConfigFiles` writes decrypted file before backfilling — concurrent readers can see the password in plaintext during the gap.**
- File: `backend/sync/crypto.go:159-193` (`decryptConnectionsFile`), line 192 writes plaintext *after* the loop at 181-188 that calls `_ = kc.SetPassword(id, pw)` and clears `pw` from the JSON. The order is correct within ONE function call, BUT `encryptConnectionsFile` encrypts BEFORE write — making local-state-at-rest clean. No issue here.
- (False positive — reorder is correct. Removing the finding.)

**SYNC-P2-6: `TestConnection` constructor builds a `*git.Remote` purely from URL — if URL is `file://` it triggers local file enumeration.**
- File: `backend/sync/git.go:254-265`.
- Scenario: User paste-types `file:///etc` as a test. `remote.List(&git.ListOptions{...})` reads the local filesystem at that path. While this requires the user to cooperate, the wider issue is that the URL goes through go-git's transport router — `ext` scheme (used by some self-hosted git repos) or `git+ssh://` would route via cmd/exec. go-git v5 is hardened against cmd injection but older vectors exist.
- Failure: Disk-disclosure if user picks a `file://` URL; no scheme allow-list.
- Fix category: Validate URL against an allow-list of schemes (`https`, `http`, `git`, `ssh`).

**SYNC-P2-7: `GetEncryptionKey` decodes hex without length validation — `aes.NewCipher` rejects short keys at use, but the error is per-call-site.**
- File: `backend/sync/keychain.go:40-46`.
- Scenario: Keychain stored value is corrupted by a partial write (keyring backend truncate on some Linux distros). `hex.DecodeString` succeeds with a 16-byte buffer. `aes.NewCipher` panics-or-errors on first encrypt, but the function call paths don't all defensively check.
- Failure: Hard-to-diagnose error surfacing at encrypt time.
- Fix category: Check `len(decoded) == keyLength` in `GetEncryptionKey` and return a clear error.

**SYNC-P2-8: `compareConfigDirs` does NOT consider file mode bits — `0600` vs `0644` would not register as different.**
- File: `backend/sync/sync_service.go:462-473`.
- Scenario: User added a script wrapper to `~/.config/uniTerm/scripts/` that the sync pulled down with world-readable perms. Subsequent syncs don't perceive perms drift.
- Failure: Permission drift goes undetected.
- Fix category: Out of scope for current data model (the sync only covers 3 known JSON files which always serialize with 0600). Logging this for future expansion.

**SYNC-P2-9: `CloneOrOpen` falls back to `initEmpty` for the EMPTY remote repository case — but it never re-tries the clone path.**
- File: `backend/sync/git.go:32-63`, particularly 56-58 (`if errors.Is(err, gittransport.ErrEmptyRemoteRepository) { return initEmpty(...) }`).
- Scenario: First sync to a newly-created GitHub repo (zero commits). `CloneOrOpen` → `PlainClone` errors `ErrEmptyRemoteRepository` → `initEmpty` creates a local repo. Next Sync, `git.PlainOpen(repoPath)` succeeds. State is consistent. *However*, the local clone did not pull from remote, so the `origin` remote's HEAD setting is missing — this propagates into later calls where `CompareHeads(branch)` looks up `refs/remotes/origin/main` and fails with `plumbing.ErrReferenceNotFound`. The code handles this (line 181-184 in git.go: returns `SyncPush`), but only for the current branch. If the user has a non-`main` branch as default, `initEmpty` always sets `refs/heads/main` as HEAD — wrong branch.
- Failure: Wrong default branch gets set as HEAD; Sync() pushes to `main` even when remote default is `master`.
- Fix category: Query remote's default branch via `git ls-remote` before deciding branch.

**SYNC-P2-10: `PushToBranch` is hardcoded to use the local branch of the same name as the target — does not handle `HEAD:refs/heads/...` semantics.**
- File: `backend/sync/git.go:217-229`.
- Scenario: After `initEmpty` sets HEAD to `main` but user has data committed under `master` (in a fresh reconfiguration scenario), `PushToBranch("main", ...)` will push from local `main` branch — which may not exist or may be the wrong branch.
- Failure: Push to wrong branch.
- Fix category: Use `HEAD:refs/heads/<branch>` refspec so any local branch state is correctly mapped.

**SYNC-P2-11: `ResolveConflict(useLocal=false)` does `ResetToRemote` (HARD reset) without backing up the un-pushed local state.**
- File: `backend/sync/sync_service.go:215-220`.
- Scenario: User has uncommitted local edits that haven't yet reached a commit (the uniterm code only StageAndCommit's after `EncryptConfigFiles` so the worktree changes are encrypted blobs — but in-progress edits during a Sync mid-flight could be wiped). Worse: if the user is in the middle of editing a connection and accidentally hits "Use remote", the `wt.Reset(&git.ResetOptions{Mode: git.HardReset})` wipes the *worktree index state* — including any pending-but-not-yet-committed local changes.
- Failure: Silent destruction of in-progress edits.
- Fix category: `git.Stash` local state before `ResetToRemote`; offer "restore on demand".

### P3 — Low / Informational

**SYNC-P3-1: `IsAutoSyncEnabled` returns a boolean but no caller invokes an auto-sync loop.**
- File: `backend/sync/sync_service.go:249-253`.
- Scenario: User enables "Auto Sync" in settings. Nothing happens. There is no caller of `Sync()` from a goroutine / timer.
- Failure: Feature claimed in UI is a no-op.
- Fix category: Either wire a periodic `time.Ticker` in `App.startup` or remove the flag.

**SYNC-P3-2: `commitMsg` uses `time.RFC3339` which omits sub-second precision — conflicting commits within the same second sort non-deterministically.**
- File: `backend/sync/sync_service.go:672-676`.
- Failure: Cosmetic — duplicate-timestamp commits are unusual but possible with rapid auto-commits.
- Fix category: Use RFC3339Nano.

**SYNC-P3-3: `keychainService = "uniTerm"` is generic — collides with other apps that pick the same service name.**
- File: `backend/sync/keychain.go:14`.
- Scenario: Another app (e.g. JetBrains' "uniTerm" plugin, or any other app whose team happened to pick the literal string "uniTerm") writes a `git-token` or `encryption-key` entry under the same OS keychain service. The two apps share keychain entries by key name (e.g. `git-token`). On Linux libsecret, key names like `git-token` are common — collision is likely.
- Failure: Silent credential cross-pollination between apps on the same machine.
- Fix category: Use a reverse-DNS or org-prefixed service name (e.g. `com.uniterm.sync`).

**SYNC-P3-4: `IsAutoSyncEnabled` reads config without `s.mu` — minor race during `SaveConfig`.**
- File: `backend/sync/sync_service.go:249-253`.
- Failure: Read may return stale value; not a correctness bug but a data race detectable by `-race`.
- Fix category: Use `sync.RWMutex` (read locks for `IsAutoSyncEnabled`, write for `SaveConfig`/`Sync`).

**SYNC-P3-5: `updateLastSyncResult` writes the config file on every Sync outcome — small IO amplification.**
- File: `backend/sync/sync_service.go:241-247`, called from many sync branches (96, 104, 110, 119, 122, 125, 132, 138, 143, 146, 151, 155, 158, 211, 223, 321, 342, 365, 424).
- Failure: 10+ writes per sync flow. Tiny but multiplied by auto-sync frequency.
- Fix category: Write once at the end of `Sync()` based on the final outcome; merge partial-write calls.

**SYNC-P3-6: `GetEncryptionKey` decodes hex without enforcing key length 32 — see SYNC-P2-7. (Cross-ref.)**

**SYNC-P3-7: `StageAndCommit` uses hardcoded author name/email `uniTerm <uniterm@local>`.**
- File: `backend/sync/git.go:105-110`.
- Failure: All sync commits share one identity. Multi-device sync loses attribution; conflict analysis can't tell which device produced which commit.
- Fix category: Source author/email from `SyncConfig` (user-managed) or generate per-install.

**SYNC-P3-8: `Sync()` always calls `CloneOrOpen` (disk-touching) even when the repo is already open — second sync re-opens.**
- File: `backend/sync/sync_service.go:94`.
- Failure: Disk I/O on every click.
- Fix category: Cache `*GitRepo` on `SyncService`; reopen only on error.

**SYNC-P3-9: `decryptBytes` accepts `additionalData nil` for AAD — see SYNC-P1-1. Documented at P1; this is the data-flow cross-ref.**

**SYNC-P3-10: `SyncConfig.LastSyncError` is a string field — CONCERNS flags it may carry credential leakage; the field has no max length.**
- File: `backend/sync/sync_config.go:19` (struct), `backend/sync/sync_service.go:241-247` (setter).
- Failure: Long or malformed errors overflow the file (mitigated by file size limits elsewhere); also raw error text pasted through `fmt.Errorf("...: %v", err)` where `err` is go-git's transport error which often contains the URL.
- Fix category: Strip URL/credentials from errors before storing; cap length to e.g. 512 chars.

**SYNC-P3-11: `keyLength = 32` and `saltLength = 16` constants are not exposed — tests can't override for fast iteration tests.**
- File: `backend/sync/keychain.go:17-19`.
- Failure: Cannot write fast unit tests for `DeriveKey` (PBKDF2 with 600k iterations blocks test suites for many seconds).
- Fix category: Refactor to dependency-inject iteration count; test-fast builds use 1k iterations.

**SYNC-P3-12: `compareConfigFiles` mentions "backfill passwords from keychain on the local side before comparison" (line 461 docstring) but the *remote* side is not similarly backfilled — comparison is asymmetric.**
- File: `backend/sync/sync_service.go:476-496`.
- Failure: If remote stored an empty password (because it relied on the keychain at encryption time), local comparison will be asymmetric. Currently mitigated because encryption always inlines the password (line 65-77 of crypto.go), so remote always has the password inline. Pre-emptive note for future migration.
- Fix category: Document and add a symmetric backfill on remote as well.

**SYNC-P3-13: `getConfigModTime` returns zero `time.Time` if no files exist — `ConflictInfo.LocalTime` JSON-serializes as `0001-01-01T00:00:00Z`, leaking that the file is absent.**
- File: `backend/sync/sync_service.go:429-441`.
- Failure: Cosmetic; sends "epoch zero" to frontend.
- Fix category: Use `*time.Time` (pointer) with `omitempty`.

---

## Cross-cutting concerns within module

- **Mutex discipline is inconsistent.** `Sync`, `ConfigureRepo`, `ChangePassword`, `DeleteRepo` all take `s.mu`, but `ResolveConflict` (which mutates the same on-disk repo) does not. Even reads (`IsAutoSyncEnabled`, `GetConfig`) bypass the lock. The unit is `sync.Mutex`, not `RWMutex`, so concurrent reads from SaveConfig-side have to wait for in-flight Sync. *(See SYNC-P0-2, SYNC-P3-4.)*

- **Encryption uses raw AES-GCM with no AAD.** Both `Seal` and `Open` pass `nil` as `additionalData`. The keychain-backed keychain pass survives, but cross-file cipher swaps are possible. *(See SYNC-P1-1.)*

- **Comparison is structural, not semantic.** `compareConfigFiles` relies on `json.Marshal` equality after one-sided keychain backfill, which is fragile under schema migrations and field-order changes. The whole "drift detection" loop in `Sync()` and `ConfigureRepo()` runs through this same weak comparison. *(See SYNC-P1-2, SYNC-P2-3.)*

- **Temp dir cleanup is `defer os.RemoveAll`.** This survives normal-return paths but **not**: panic during `EncryptConfigFiles`, process kill (SIGKILL), or OS-level crash. The decrypted-on-disk window is real for the duration of the Sync() call. CONCERNS already flags this; cross-ref SYNC-P1-11.

- **Username-as-displayed-elsewhere.** `config.Username` is used as the BasicAuth username (which is fine for `username:token` form), as a GitHub-style personal access token user (also fine), but it is ALSO persisted in plaintext `sync-config.json` and visible to anyone with config-dir read access. *(See SYNC-P1-7.)*

- **Frontend error surfacing amplifies Sync bugs.** Sync() result is bubbled to frontend with the wrapped error string (CONCERNS already calls out for Basic-Auth). Many of the above findings (ResolveConflict race, hostname leak, transient network) only become user-visible *because* the error is forwarded as-is.

- **No file-size / progress / abort signal.** Long-running Push/Pull operations block the entire UI thread. A 100MB `connections.json` (unusual but possible if a user stores terminal output snippets there) would freeze the UI with no cancellation. *(See SYNC-P1-10.)*

---

## Summary
- **Total findings:** 25 (P0: 2, P1: 11, P2: 9, P3: 13 — P2-5 retracted as false positive)
- **Confidence:** high for P0/P1 (line-precise reproductions); medium for P2/P3 (some are speculative, flagged for further review)

### Top 5 to fix first
1. **SYNC-P0-1** — Multi-file emptiness check (can destroy settings on first sync).
2. **SYNC-P0-2** — `ResolveConflict` mutex missing (data corruption on rapid UI clicks).
3. **SYNC-P1-11** — Decrypt failures are silently treated as "needs commit", can overwrite remote with local → permanent remote data loss on bad password.
4. **SYNC-P1-1** — AES-GCM has no AAD → cross-file ciphertext swap is undetectable.
5. **SYNC-P1-4** — Hostname leak in commit messages (privacy / fingerprinting in shared org repos).

---

## Audit notes (methodology, reproducibility)
- Read order: CONCERNS.md → all 5 source files → targeted grep for `exec.Command`, `rand.Read`, `pbkdf2`, `nonce`, `ForcePush`, `ResetToRemote`, `os.MkdirTemp`, `Mutex`.
- Each finding cites file:line, gives the *exact* triggering state/input, and states the *observable wrong output*. None requires constructing a full end-to-end test; the line numbers were verified by direct Read.
- Did NOT modify any source. Output is at `/Users/coderstory/CodeSource/uniterm/.planning/audit/phase-1/backend-sync.md`.







