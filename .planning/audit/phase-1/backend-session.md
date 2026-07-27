# Phase 1 Audit — backend/session/

**Auditor:** gsd-code-reviewer (Phase 1)
**Date:** 2026-07-28
**Scope:** `backend/session/*.go` (protocol sessions, manager, tunnel, output_log, post_login, zmodem, monitor, proxy)

---

## Findings (by severity)

### P0 — Critical

(none)

---

### P1 — High

**AUDIT-session-01: SSH `Disconnect` closes `quit` channel in `sync.Once`, making subsequent `Connect` calls on the same `SSHSession` silently broken.**
- File: `backend/session/ssh_session.go:466-478` (Disconnect), `backend/session/ssh_session.go:53-62` (NewSSHSession), `backend/session/ssh_session.go:419-442` (startKeepAlive).
- Severity: P1
- Failure scenario: User clicks "reconnect" in the UI on a panel that holds an existing `SSHSession` (created earlier via `SessionManager.Create`). The frontend calls `Connect()` again on the same `Session` instance. `NewSSHSession` has already closed `quit` on first `Disconnect`, so:
  - `startKeepAlive` exits on its first select iteration (`case <-s.quit:` is already closed) → no keepalive is sent → connection drops after 60 s of idle or a NAT idle timeout.
  - `readLoop` and `readStderr` are launched, but `s.stdout`/`s.stderr` from the closed session are EOF; both goroutines immediately re-enter `Disconnect()` (no-op via `quitOnce`) and return. The "reconnected" panel shows empty output and no keepalive.
  - The user sees a frozen terminal until they manually close the tab.
- The deferred `s.authAnswerCh = nil` at lines 85-89 also leaves `kbCallback` reading from a now-dangling `s.authAnswerCh` (the field was set at line 82 but the deferred nil-out races with the callback reading it through the `s.` field after the handshake error).
- Fix category: Re-create `s.quit`, `s.authAnswerCh`, `s.expectOutput` and reset `s.quitOnce` (`sync.Once` is not resettable, so replace with a fresh field) inside `Connect`, or document that `SSHSession` is single-use and have `SessionManager.Create` always return a fresh instance.
- Evidence:
  ```go
  // ssh_session.go:53-62
  func NewSSHSession(id string) *SSHSession {
      return &SSHSession{
          baseSession: baseSession{...},
          quit: make(chan struct{}),  // created once; never recreated
      }
  }
  // ssh_session.go:466-477
  func (s *SSHSession) Disconnect() error {
      s.quitOnce.Do(func() {
          close(s.quit)            // permanently closes quit for the lifetime of this SSHSession
          ...
      })
      return nil
  }
  ```

**AUDIT-session-02: `s.authAnswerCh = nil` defer races with the `kbCallback` goroutine reading `s.authAnswerCh` after Connect returns.**
- File: `backend/session/ssh_session.go:80-89, 127-168` (kbCallback reads `s.authAnswerCh` directly through the receiver pointer).
- Severity: P1
- Failure scenario: User clicks Cancel during keyboard-interactive password prompt. `kbCallback` is mid-loop on `<-s.authAnswerCh` (line 135) when the auth method errors out (because `Disconnect` closed the SSH client). At the same time, `Connect`'s deferred cleanup at lines 85-89 sets `s.authAnswerCh = nil`. If `kbCallback` runs again (e.g. server retries the auth method, which some PAM configs do), it reads the nil channel and **blocks forever** — the goroutine leaks and the SSH handshake never returns.
- The `Write` method (line 444-460) properly guards with `s.mu.RLock()` so it sees `nil` and falls through; but `kbCallback` is a closure that touches the field without re-locking on each iteration. This is a real Go race detector hit.
- Fix category: Have `kbCallback` capture the channel as a local variable (`ch := s.authAnswerCh` under the same lock as line 82) instead of dereferencing through `s.` on every iteration. Or hold the channel until the SSH handshake fully returns, then nill it.
- Evidence:
  ```go
  // ssh_session.go:127-167
  kbCallback := func(user, instruction string, questions []string, echos []bool) ([]string, error) {
      ...
      for {
          select {
          case data := <-s.authAnswerCh:   // accesses s.authAnswerCh through receiver
              ...
  ```
  The `defer` at lines 85-89 mutates the same field after Connect returns; the closure continues to read it.

**AUDIT-session-03: FTPSession `s.conn` is closed in `Disconnect` without holding `s.connMu`; concurrent FTP operations in transfer goroutines race with the close.**
- File: `backend/session/ftp_session.go:112-119` (Disconnect), `backend/session/ftp_session.go:147-178, 285-340, 439-475, 498-540, 609-680` (call sites that lock `s.connMu`).
- Severity: P1
- Failure scenario: User starts a long download via `Get` recursive (line 285). Inside the launched goroutine, `s.startTransfer` is called (line 339) which acquires `s.connMu` at line 628. Meanwhile, the user clicks Disconnect (or the connection times out). `Disconnect` at line 114 calls `s.conn.Quit()` and `s.conn = nil` WITHOUT acquiring `s.connMu`. The `jlaffaye/ftp` library is not safe to call `Quit` and `List`/`Retr` concurrently; concurrent call typically panics on `send / receive on closed channel` from inside the library's internal mutex. The user sees the app crash with the FTP goroutine stack at the top.
- Fix category: Move `s.conn` reads/writes under `s.connMu`; have `Disconnect` take `s.connMu` before `Quit()` (acquire it after deleting from transfers, since the lock is held during normal ops).
- Evidence:
  ```go
  // ftp_session.go:112-119
  func (s *FTPSession) Disconnect() error {
      if s.conn != nil {       // unsynchronized read
          s.conn.Quit()        // unsynchronized call concurrent with locked callers
          s.conn = nil         // unsynchronized write
      }
      s.setStatus(StatusDisconnected)
      return nil
  }
  ```
  vs. transfer goroutine at ftp_session.go:628-679 which acquires `s.connMu`.

**AUDIT-session-04: VNCProxy/ SPICEProxy `Stop` releases the mutex and calls `p.wg.Wait()`, but a `handleWebSocket` invocation that races into the `p.wg.Add(2)` window between Stop's `p.listener.Close()` and `p.wg.Wait()` adds goroutines the Wait does not see.**
- File: `backend/session/vnc_proxy.go:148-162` and `backend/session/spice_proxy.go:151-165` (identical structure).
- Severity: P1
- Failure scenario: A browser tab is mid-upgrade when the user clicks Disconnect. `Stop()` runs: close `stopCh`, lock `p.mu`, close `p.wsConn`, close `p.tcpConn`, close listener. `p.wg.Wait()` returns because the old 2 goroutines exited. Between `p.listener.Close()` and `p.wg.Wait()` returning, the http handler that was just dispatched (before listener close took effect) successfully upgrades a WebSocket and enters `handleWebSocket`, which calls `p.wg.Add(2)` and launches 2 new goroutines. `p.wg.Wait()` then returns with `Wait()` having seen only the OLD counter — the new 2 are unaccounted for. The new goroutines run until their first TCP/WS error, but `p.listener`, `p.wsConn`, etc. are now in undefined state. Goroutine leak and use-after-close on `p.tcpConn` / `p.wsConn`.
- Fix category: Move `p.listener.Close()` BEFORE the `p.wg.Wait()` and add a final atomic/flag check in `handleWebSocket` that refuses to start if Stop has been signalled (use the existing `p.stopCh` select).
- Evidence:
  ```go
  // vnc_proxy.go:148-162
  func (p *VNCProxy) Stop() {
      p.stopOnce.Do(func() { close(p.stopCh) })   // (1) signal
      p.mu.Lock()
      if p.wsConn != nil { p.wsConn.Close() }
      if p.tcpConn != nil { p.tcpConn.Close() }
      p.mu.Unlock()
      if p.listener != nil { p.listener.Close() } // (2) listener closed here
      p.wg.Wait()                                  // (3) — but http.Serve may still dispatch a handler between (2) and (3)
  }
  ```

**AUDIT-session-05: Telnet auto-login sends `password + "\r\n"` on a 1.5 s/1.2 s fixed `time.Sleep` schedule; if the remote delays its login prompt beyond ~1.5 s the username is sent before the prompt, often landing in the shell prompt as garbage.**
- File: `backend/session/telnet_session.go:234-258`
- Severity: P1
- Failure scenario: A telnet BBS / old Cisco IOS device is slow to emit its `Username:` prompt after IAC negotiation (or has a banner). `time.Sleep(1500ms)` expires and `s.conn.Write([]byte(user + "\r\n"))` runs. The username ends up after the prompt, often preceded by echo of empty characters and the user's typed password. The shell then either accepts garbage as commands (security exposure: any command the username string contains, e.g. `admin;reboot`, runs on the device) or rejects the connection. The 1200 ms password delay compounds — by the time the password is sent the shell may have already exited, sending the password into a closed connection.
- Fix category: Replace `time.Sleep` with a prompt-detect loop using `filterIAC`'s readable state, or wait for a configurable banner timeout. The existing `filterIAC` already handles WILL/DO/SB — extend it to surface a "banner seen" signal.
- Evidence:
  ```go
  // telnet_session.go:234-258
  func (s *TelnetSession) sendAutoLogin(ctx context.Context, user, password string) {
      time.Sleep(1500 * time.Millisecond)
      ...
      if s.conn != nil {
          s.conn.Write([]byte(user + "\r\n"))
      }
      if password != "" {
          time.Sleep(1200 * time.Millisecond)
          ...
          s.conn.Write([]byte(password + "\r\n"))
      }
  }
  ```
  No prompt detection; sleeps are hard-coded.

---

### P2 — Medium

**AUDIT-session-06: SSH `kbCallback` reads user input that includes remote echo bytes (e.g. MOTD) without filtering.**
- File: `backend/session/ssh_session.go:94-168`
- Severity: P2 (related to CONCERNS "SSH keyboard-interactive prompt silently disables echo for password input but uses ASCII printable for echo-on prompts")
- Sharpen: CONCERNS already documents this for kbCallback. This audit confirms the bug extends to the OUTER pre-prompt password loop at lines 96-122 (the `shouldPromptForSSHPassword` path). There is no `echos[]` array (only the kbCallback variant has it), so the loop accumulates EVERY byte sent by the remote into `answer`. If the remote's banner (which usually arrives during handshake) bleeds into the authAnswerCh after the loop starts, the password gets prepended with banner garbage. Reproduced by reading lines 96-122: the `case data := <-s.authAnswerCh` default branch appends `b` to `answer` without checking if the bytes came from `s.authAnswerCh` (user input) or from `expectOutput` (remote output) — note `authAnswerCh` is fed only by `Write`, but `Write` may be called by the frontend during the auth window if the frontend mistakenly writes anything to a non-yet-connected tab.
- Fix category: Use the same `isTTY` / echo-on/off split as kbCallback; ignore bytes received before the user actually starts typing.

**AUDIT-session-07: `output_log.go` `WriteOutput` calls `file.Sync()` after EVERY write.**
- File: `backend/session/output_log.go:522-524`
- Severity: P2 (already in CONCERNS as "output_log.go flushes on every write")
- Sharpen: Affects `applyCSI`'s `default` switch (line 233-246) for CSI P (`DCH`) — `copy(p.line[p.pos:], p.line[p.pos+n:])` is correct, but if `p.pos+n > len(p.line)` the underflow check at line 309-314 silently truncates without resetting `p.emitted`. After truncate, `p.emitted` could be > `len(p.line)`, and the flush check at line 325-327 clamps it — OK. Real edge case: CSI G/` (CHA) at line 277-286: if `col > len(p.line)`, `col = len(p.line)`, then `p.pos = col`; a subsequent write at `p.pos` is `default:` path, line 234-235 `p.line[p.pos] = b` — fine.
- The actual perf cost: `file.Sync()` per write at line 523 is the dominant issue. Confirming CONCERNS, no new bug.
- Fix category: Buffer in `bufio.Writer` with a 4 KiB / 100 ms flush; only `Sync` on Disable.

**AUDIT-session-08: `output_log.go` `lineProcessor` `applyCSI` for ICH (`@`) at line 315-321 does not preserve already-flushed bytes when inserting blanks at the cursor.**
- File: `backend/session/output_log.go:315-321`
- Severity: P2
- Failure scenario: A TUI emits `ESC[2@` (insert 2 blanks) at position 5 while `p.emitted = 8` (i.e. 8 bytes already flushed). The code does:
  ```go
  ins := make([]byte, n)
  p.line = append(p.line[:p.pos], append(ins, p.line[p.pos:]...)...)
  ```
  This inserts blanks AT the cursor, pushing the existing content right. But `p.emitted` stays at 8 — so the next flush writes `p.line[p.emitted:]` which now starts at byte index 8 but the byte at index 8 is one of the freshly-shifted bytes (which was originally at index 6). The flushed log now contains a byte that was supposed to be from the original `p.line[6]` twice in slightly different context — visual log corruption.
- Fix category: When inserting blanks that shift already-emitted bytes right, increment `p.emitted` by `n` (so the shifted bytes are not re-flushed). When inserting blanks that overlap not-yet-emitted bytes, leave `p.emitted` alone.
- Evidence:
  ```go
  // output_log.go:315-321
  case '@': // ICH — insert n blank characters at cursor
      n := parseCSIParam(params, 0, 1)
      if p.pos >= len(p.line) {
          return
      }
      ins := make([]byte, n)
      p.line = append(p.line[:p.pos], append(ins, p.line[p.pos:]...)...)
      // BUG: p.emitted is not adjusted for the inserted blanks
  ```

**AUDIT-session-09: `postLoginOutputBuffer.trimPostLoginOutput` slices at byte boundary, can split a multibyte UTF-8 rune.**
- File: `backend/session/post_login_expect.go:185-190`
- Severity: P2
- Failure scenario: An expect step waits for a prompt that contains CJK characters (e.g. `请输入密码:`). The server sends a long MOTD followed by the prompt, total > 8192 bytes. The trim at line 189 keeps `output[len-8192:]`, which may start at a continuation byte (0x80-0xBF). `matchesPostLoginExpect` then does `strings.ToLower(output)` and `strings.Contains(...)`. `strings.Contains` operates on bytes — if the prompt's first byte is a continuation byte, the substring search fails to find it, and the expect times out even though the prompt was sent.
- Fix category: Find the next UTF-8 start byte (one where `b < 0x80 || b >= 0xC0`) and trim from there.
- Evidence:
  ```go
  // post_login_expect.go:185-190
  func trimPostLoginOutput(output string) string {
      if len(output) <= postLoginExpectBufferLimit {
          return output
      }
      return output[len(output)-postLoginExpectBufferLimit:]  // may split UTF-8
  }
  ```

**AUDIT-session-10: `monitor_session.go` `pushSystemInfo` is fire-once at Connect; on any transient SSH error during `collectSystemInfo`, the system info is never sent.**
- File: `backend/session/monitor_session.go:151, 159-227`
- Severity: P2
- Failure scenario: MonitorSession is created while the target's SSH daemon is under CPU pressure (e.g. just rebooted). `pushSystemInfo` is launched, calls `s.client.NewSession()` → returns `*ssh.Session`, runs `CombinedOutput(script)`. The script (line 177) reads `/etc/os-release` via `grep`/`awk`. Under high load, the SSH session creation succeeds but `CombinedOutput` returns an error after some seconds (or `NewSession()` itself returns an error, which is swallowed at line 173). The monitor panel never shows the OS / kernel / CPU info, and there is no retry. User has to close and recreate the monitor session.
- Fix category: Wrap `pushSystemInfo` in a retry loop with backoff; or schedule it from `pollLoop` once per N ticks (so transient errors recover).
- Evidence:
  ```go
  // monitor_session.go:159-168
  func (s *MonitorSession) pushSystemInfo() {
      info := s.collectSystemInfo()
      if info != nil { ... }
      // no retry, no error log
  }
  // monitor_session.go:151
  go s.pushSystemInfo()   // fired once, no recovery
  ```

**AUDIT-session-11: `tunnel_forward.go` `StartTunnel` does not call `tunnelKeepAlive` if the listener is created but the `startListener` step fails after `dialChain` succeeds.**
- File: `backend/session/tunnel_forward.go:48-76`
- Severity: P2
- Failure scenario: User tunnel dials chain successfully (line 48), then `startListener` fails because the chosen local port is busy (line 54-58). `closeClients(clients)` is called, error returned. All clients are closed; no goroutine was launched. But the `clients` slice may have had partial state where `prev` was set — actually `closeClients` iterates and closes. Looks OK here.
- A subtler case: `startListener` succeeds, line 60-62 stores the entry, line 66 launches `tunnelKeepAlive`. If `tunnelKeepAlive` itself panics (e.g. `exit` is nil because `dialChain` returned `(nil, clients, nil)` for a chain length 0 — but `buildHopChain` rejects empty exitID), then `exit.SendRequest` crashes. Currently the function returns before launching if the entry is invalid; but no recovery. P3-level safety issue.
- Fix category: Wrap `go tunnelKeepAlive(exit, quit, ...)` in a recover-and-log. Also wrap `go func() { exit.Wait(); ... }` (line 67-76) in case `exit` is nil.

**AUDIT-session-12: `tunnel_service.go` `Shutdown` and `Stop` close `entry.sshClient` after closing the listener; but a slow `sshClient.Close()` blocks the Shutdown for many concurrent tunnels.**
- File: `backend/session/tunnel_service.go:164-177, 179-194`
- Severity: P2
- Failure scenario: App quit calls `Shutdown()`. For each tunnel, it does `entry.sshClient.Close()` synchronously (line 192). `ssh.Client.Close()` blocks until all in-flight requests are drained and the connection is closed by the remote — up to the SSH timeout (typically 30 s). With N tunnels open, app shutdown is delayed by up to N*30 s. Wails can SIGKILL the process during quit, leading to leaked sockets and possible "address already in use" on the next launch.
- Fix category: Close listener first, then call `sshClient.Close()` in a goroutine; or use `client.Close()` in a goroutine with a WaitGroup and cap the shutdown wait at e.g. 2 s.

**AUDIT-session-13: Mosh server parser uses `bufio.Scanner` with default 64 KiB token size; no `scanner.Err()` check.**
- File: `backend/session/mosh_session.go:138-149`
- Severity: P2
- Failure scenario: `mosh-server new -s` output, when the server's key is unusually long or the path message includes base64 with embedded newlines, may produce a single line longer than 64 KiB. `bufio.Scanner` silently truncates with `bufio.ErrTooLong` set on `scanner.Err()`. We never check it. `output.String()` returns only the prefix; `MOSH_KEY=` or `MOSH_PORT=` line is incomplete; `startMoshServer` returns "missing key or port" — but the user sees a generic failure with no log of the actual scan error.
- Fix category: Use `bufio.NewScanner(stdout)` with `scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)` and `if err := stdoutScanner.Err(); err != nil { log... }`.
- Evidence:
  ```go
  // mosh_session.go:139-149
  stdoutScanner := bufio.NewScanner(stdout)
  for stdoutScanner.Scan() {
      output.WriteString(stdoutScanner.Text())
      output.WriteByte('\n')
  }
  // no check of stdoutScanner.Err()
  ```

**AUDIT-session-14: `SerialSession.readLoop` does not check `s.quit`; relies solely on `s.port.Read` returning an error on close.**
- File: `backend/session/serial_session.go:109-136`
- Severity: P2
- Failure scenario: A USB-serial adapter that buffers internally or whose driver does not propagate close to a Read EOF — the `readLoop` blocks indefinitely even after Disconnect. Disconnect sets `s.port.Close()` but if the kernel driver holds the FD open, the goroutine leaks. Subtle and driver-specific.
- Fix category: Add `select { case <-s.quit: return; default: }` before `s.port.Read`, or use a `SetReadDeadline`-based poll loop.

**AUDIT-session-15: `FTPSession.ChangeRemoteDir` checks directory existence via `s.conn.List(target)` then sets `s.cwd = target` (line 200-206) WITHOUT `s.connMu`, while other code paths hold `s.connMu`.**
- File: `backend/session/ftp_session.go:191-223`
- Severity: P2
- Failure scenario: User navigates directories rapidly. Concurrent `Get` calls are holding `s.connMu`. `ChangeRemoteDir` (called from the frontend binding) bypasses the mutex and calls `s.conn.List(target)`. The `jlaffaye/ftp` library uses internal locks per ServerConn, but some calls (e.g. `cwd` bookkeeping) are not serialized — concurrent `List` and `Quit` (from `Disconnect`) can corrupt internal state, leading to a subsequent FTP command returning "internal error" or hanging.
- Fix category: Wrap `ChangeRemoteDir`'s `s.conn.List(target)` in `s.connMu` like `ListRemote` does at line 151.

**AUDIT-session-16: `ssh_session.go` `decodeOutput` holds `s.mu` for the entire Transform loop, which can be tens of ms on large chunks.**
- File: `backend/session/ssh_session.go:512-535`
- Severity: P2
- Failure scenario: For GBK/Big5 sessions (Chinese / Japanese / Korean servers), each readLoop chunk (up to 4096 bytes) holds the session mutex while running `encoding.Decoder.Transform`. While the lock is held, `Write` (line 444), `Disconnect` (line 466), and other session-level operations block. On a slow CPU this manifests as sluggish typing on a Chinese session.
- Fix category: Use a per-decoder mutex separate from `s.mu`; only sync on the `s.decoder`/`s.decodeLeftover` reads/writes. Document that `decodeOutput` is single-goroutine in a comment (already stated) — the lock is only needed for the field reads, not the Transform.

**AUDIT-session-17: `ssh_auth.go` `parsePrivateKeyFile` swallows ALL errors (`return nil, false`).**
- File: `backend/session/ssh_auth.go:33-50`
- Severity: P2
- Failure scenario: User points `KeyPath` at a file with restrictive permissions (e.g. `0644`); `ssh.ParsePrivateKey` succeeds but later `ssh.PublicKeys` rejects with a generic "Permissions 0644 for '/path/key' are too open" message that surfaces to the UI. The user has no way to know whether the file is missing, malformed, or wrong permissions. Worse: `ssh.ParsePrivateKeyWithPassphrase` returns a specific error for wrong passphrase (`*ssh.PassphraseMissingError` etc.) that is collapsed into the same `nil, false`.
- Fix category: Return the actual error wrapped (`return nil, false, fmt.Errorf("read key %s: %w", path, err)`), or at least `var reason error` so the caller can surface it. The `ssh_session.Connect` is the caller.

**AUDIT-session-18: `output_log.go` `Enable` writes banner with `_, _ = fmt.Fprintf` ignoring write errors; `WriteOutput` swallows errors from `file.Write`.**
- File: `backend/session/output_log.go:452-453, 461-463, 477-478, 505-524`
- Severity: P2
- Failure scenario: Log directory is on a read-only mount or NFS share with stale handle. `os.MkdirAll` (line 416) succeeds (assumes parent exists) but `os.OpenFile` later fails. The caller sees a non-nil error from `Enable`. But during `WriteOutput` (line 522-524), if `file.Write` fails (disk full, NFS stale), the error is dropped — `Sync()` is skipped, the user has no signal that the log is silently failing. Long-running sessions can lose the entire log without anyone noticing.
- Fix category: Increment a `writeErrors uint64` counter; surface via a method on `OutputLogger` so the App layer can warn. Or call `log.Writef("output log write failed: %v", err)` and silently disable subsequent writes to avoid spinning on a broken file.

---

### P3 — Low / Informational

**AUDIT-session-19: `LocalSession.Disconnect` (`local_session_unix.go:193-205`) is not wrapped in `sync.Once` for `s.pty.Close()` / `s.cmd.Process.Kill()`.**
- File: `backend/session/local_session_unix.go:193-205`, `backend/session/local_session_windows.go:366-382`
- Severity: P3
- Failure scenario: Wait goroutine at line 121-124 calls `Disconnect()` after process exits; user-initiated Disconnect calls it concurrently. The Unix variant closes `s.quit` under `quitOnce.Do` (good) but then calls `s.pty.Close()` and `s.cmd.Process.Kill()` outside the once — second call's `Process.Kill()` returns "process already finished" error which is silently dropped (line 201). `setStatus(StatusDisconnected)` runs twice, firing the status callback twice (the App layer should be idempotent; minor).
- The Windows variant (line 366-382) wraps EVERYTHING in `disconnectOnce.Do`, so this is fixed there.
- Fix category: Move `s.pty.Close()` and `s.cmd.Process.Kill()` inside the `quitOnce.Do` block on Unix, mirroring the Windows variant.

**AUDIT-session-20: `manager.go` type-based switch has 20+ cases — adding a new protocol requires editing both `manager.go` and `app.go`.**
- File: `backend/session/manager.go:21-84`
- Severity: P3 (already noted in CONCERNS as "Fragile Areas")
- Confirming CONCERNS. No new evidence.

**AUDIT-session-21: `SessionManager.sessions` is unbounded — long-lived apps accumulate sessions that leak when tabs are closed without `CloseSession`.**
- File: `backend/session/manager.go:10-19, 86-90` (`Add` never checks for existing)
- Severity: P3 (already noted in CONCERNS as "Scaling Limits / SessionManager map grows unbounded")
- Sharpen: `Add` (line 86-90) just inserts, allowing duplicate entries if `Add` is called twice with the same `Session.ID()` (e.g. on a reconnect that builds a new SSHSession with the same id). The duplicate would be silently overwritten — no error surfaced. The leak risk: any UI bug that calls `Create` without calling `Close` adds to the map. With UUID v4 IDs collisions are astronomically unlikely, but a misbehaving frontend could create many panels with the same logical id and grow the map.

**AUDIT-session-22: `TelnetSession.Disconnect` and `Disconnect`-equivalent paths don't call `s.conn = nil` after Close, so subsequent `Write` calls hit a closed connection and return an error rather than a clean "not connected".**
- File: `backend/session/telnet_session.go:276-288`
- Severity: P3
- Failure scenario: User types into a telnet panel after Disconnect has been called (race window before the UI updates). `Write` calls `s.conn.Write(data)` on a closed `net.Conn`; returns `*net.OpError` with `use of closed network connection`. The UI shows a generic error rather than "session disconnected". The same pattern applies to `MoshSession` and several file-transfer sessions.
- Fix category: Set `s.conn = nil` after `Close()` and have `Write` return `fmt.Errorf("not connected")` (mirroring the SSH convention in ssh_session.go:453-455).

**AUDIT-session-23: `S3Session.Disconnect` (s3_session.go:64-69) just nils the client pointer — does NOT call `s.s3 = nil` symmetrically for `bucket`; subsequent operations on the bucket name leak the previous bucket label.**
- File: `backend/session/s3_session.go:64-69`
- Severity: P3
- Failure scenario: After Disconnect, `s.bucket` retains the previous value. If a `ListRemote` is somehow invoked (it checks `s.s3 != nil` first via `requireClient`), it correctly returns an error, so this is mostly cosmetic. But `IsConnected` (line 71-73) checks `s.s3 != nil`, which is set to nil — so IsConnected returns false correctly. No security impact.
- Fix category: Clear `s.bucket = ""` only — already done (line 66). So this is correct. Listing here for completeness; not a real bug.

**AUDIT-session-24: `K8sExecSession` (k8s_exec_session.go) has no reconnect — Disconnect permanently closes the websocket; the only way to recover is to recreate the entire session via `DialExec` in `app.go`.**
- File: `backend/session/k8s_exec_session.go:71-78`
- Severity: P3
- Failure scenario: kubectl exec websocket drops (network blip, API server restart). `s.quitOnce.Do` runs `s.conn.Close()` once. `s.conn` is not nil'd, so any future `Write` returns `*websocket.CloseError`. The readLoop is already exited. The frontend sees a permanent disconnect; it must trigger `DialExec` again on the App layer. No automatic recovery.
- Fix category: Nil `s.conn` after Close; surface a typed error so the App layer can distinguish "websocket dropped" from "explicit close" and trigger a reconnect.

**AUDIT-session-25: `postLoginExpectAutomation` in `post_login_expect.go` line 87-131 does not pass `IsConnected` check at the start of each step for the FIRST iteration's `waitForPostLoginExpect`.**
- File: `backend/session/post_login_expect.go:104-129`
- Severity: P3
- Failure scenario: `IsConnected()` is checked at line 106 BEFORE the wait, but `waitForPostLoginExpect` internally calls `isConnected()` (line 151) only AFTER the first early-return check at line 141. So the first `waitForPostLoginExpect` returns immediately if `matchesPostLoginExpect(...)` is true at line 141 WITHOUT checking `isConnected()`. If the session has already disconnected between the outer check at line 106 and the inner wait call at line 114, the automation sends a reply into a closed session. Minor; the Send function (line 124) will see the closed stdin and return an error.
- Fix category: Check `isConnected()` at line 141 inside `waitForPostLoginExpect` BEFORE the initial match check.

**AUDIT-session-26: `session.go` `RunPostLoginScript` (session.go:347-378) sends each line as `line + "\r"` regardless of the OS shell. Windows local shell expects `"\r\n"` (already run through ConPTY which translates LF to CRLF), but a custom non-PTY command could lose the LF.**
- File: `backend/session/session.go:372`
- Severity: P3
- Failure scenario: Not currently triggered because `LocalSession.runPostLoginScript` sends via `s.pty.Write` which is line-disciplined. But for SSH `runPostLoginScript`, the remote is usually a Unix PTY which accepts `\r` alone. Minor; documenting for completeness.

**AUDIT-session-27: `monitor_session.go` `Disconnect` (line 1378-1390) calls `s.ticker.Stop()` AFTER `close(s.quit)`. A second call to Disconnect after the first triggers `ticker.Stop` again (no-op, idempotent) and `s.client.Close()` again (no-op, idempotent). Status callback fires twice — minor cosmetic issue.**
- File: `backend/session/monitor_session.go:1378-1390`
- Severity: P3
- Failure scenario: App layer calls Disconnect from a UI button AND from a `client.Wait()` goroutine concurrently. Both run; `setStatus(StatusDisconnected)` fires twice → UI may briefly flicker status. No correctness impact.
- Fix category: Move `ticker.Stop()` and `client.Close()` inside the `quitOnce.Do`, similar to the SSH session pattern.

**AUDIT-session-28: `output_log.go` `parseCSIParam` does not validate the numeric range; absurd CSI params like `ESC[999999999C` could OOM the line processor by advancing cursor far past the buffer.**
- File: `backend/session/output_log.go:332-362, 264-275`
- Severity: P3
- Failure scenario: A TUI sends `ESC[2000000000C` (CUF 2 billion). `parseCSIParam` returns 2000000000. `applyCSI` CUF at line 264-269: `p.pos += n`; then clamps to `len(p.line)`. So the cursor is correctly clamped. No OOM. But the partial overflow protection at line 267-269 uses an `if p.pos > len(p.line)` clamp AFTER the add — if `p.pos` is a small int and `n` overflows, the result wraps to negative, then the comparison `p.pos > len(p.line)` is false. Wait, line 264: `p.pos += n` — Go's int overflow wraps silently. With p.pos = 5 and n = 9223372036854775807 (math.MaxInt64), p.pos becomes -9223372036854775804 (negative). Then line 267 checks `p.pos > len(p.line)` — false because p.pos is negative. The cursor is now at a negative position, and subsequent bytes via `default:` (line 233) check `p.pos < len(p.line)` (true), so `p.line[p.pos] = b` PANICS with index out of range.
- Fix category: Cap `n` at a reasonable max (e.g. 4096 — line processor is for human-readable logs, not screen-sized state) in `parseCSIParam`.

**AUDIT-session-29: `tunnel_forward.go` `acceptLoop` does not respect `quit` if `ln.Accept()` is blocking when Stop is called.**
- File: `backend/session/tunnel_forward.go:321-333`
- Severity: P3
- Failure scenario: `StopTunnel` calls `entry.listener.Close()` (line 169) which makes the blocked `Accept()` return `net.ErrClosed`. `acceptLoop` checks `<-quit` at line 326 — but the Accept error already returned. If the listener has a bug where Close() doesn't unblock Accept (some custom listeners), `acceptLoop` hangs. Also: `go handle(conn)` at line 331 spawns a new goroutine for each accepted connection with no upper bound — a remote attacker can flood the listener with SYN packets and exhaust memory.
- Fix category: Wrap the listener in a `net.Listener` that returns promptly on Close; add a max-pending-connections cap (`SetListenBacklog` or accept-timeout).

**AUDIT-session-30: `manager.go` `Close` (line 92-102) calls `s.Disconnect()` AFTER releasing the lock — error from Disconnect is returned to caller but the session has been removed from the map regardless.**
- File: `backend/session/manager.go:92-102`
- Severity: P3
- Failure scenario: A buggy `Disconnect` could leave the session in `StatusConnected` after Close returns. The map no longer has it, so future calls to `Get(id)` return `not found`. No leak; no spurious operation. OK behavior.
- Confirming existing design.

**AUDIT-session-31: `session.go` `disconnectNotice` includes local time with `time.Now().Format(...)` — user's clock skew could mislead diagnostics.**
- File: `backend/session/session.go:23-26`
- Severity: P3
- Failure scenario: User's laptop clock is 3 hours off (DST change just happened, NTP not yet sync'd). The disconnect banner says "Connection closed (2026-07-28 03:00:00)" which is confusing for the user when the actual disconnect was 6 hours ago. Cosmetic.
- Fix category: Optional — format relative time ("3 minutes ago") or include both local and UTC.

---

## Cross-cutting concerns within module

1. **Lifecycle asymmetry — `sync.Once` for `close(quit)` but not for related cleanup.** Multiple sessions (SSH, Mosh, Telnet, Local-Unix, Serial, K8sExec) close `quit` under `quitOnce.Do` but then do `s.session.Close()`, `s.client.Close()`, `s.port.Close()` OUTSIDE the once. The Windows LocalSession puts everything inside `disconnectOnce.Do`. The Unix variant should follow suit (AUDIT-session-19).

2. **File-transfer sessions (`SFTP`, `FTP`, `SMB`, `WebDAV`, `S3`) all re-implement transfer task tracking independently.** Each has its own `transfers map`, `taskSeq atomic.Int64`, `Pause/Resume/Cancel` semantics, and `progressReader` wrapper. The `TransferTask` struct lives in `sftp_session.go:159-171` but is reused across all five (FTP, SMB, WebDAV, S3). The duplication is itself a maintenance hazard — pause/resume is particularly divergent (FTP line 536-538 re-creates `pauseCh` without nil-ing the old one; SFTP line 850-852 same; SMB and WebDAV copies the same pattern). PLAUSIBLE: SMB's `CancelTransfer` doesn't actually cancel the running transfer (no context plumbing through `transferFile`); a paused SMB transfer cannot be cancelled.

3. **`_ = ...` error-swallowing is pervasive for "best effort" cleanup** (e.g. `output_log.go:452-453, 463, 480-481, 523`; `local_session_windows.go:59-60`; `mosh_session.go:123`; `ssh_session.go:269`). For best-effort cleanup this is acceptable; for state-affecting calls it is not (e.g. `ssh_session.go:269` silently ignores `WindowChange` errors — terminal ends up with wrong dimensions, but no log).

4. **`time.After` is used in auth loops without draining the timer.** `ssh_session.go:118, 159` use `time.After(120s)` which leaks the timer if the auth completes before the timeout (Go FAQ: prefer `time.NewTimer` + `defer timer.Stop`). For a 120 s timer the leak is negligible, but it's a pattern other code copies (PLAUSIBLE).

5. **`strings.Contains(strings.ToLower(...), strings.ToLower(...))` is locale-unsafe** in `post_login_expect.go:171` for prompts containing Turkish 'I' / German 'ß'. Not a P0/P1 but worth flagging.

6. **All SSH-family sessions share `ssh.InsecureIgnoreHostKey()` and the legacy KEX list.** Already documented in CONCERNS. This audit confirms the same pattern appears in `tunnel_forward.go:230`, `monitor_session.go:136`, `sftp_session.go:83`, `mosh_session.go:52`, in addition to the ssh_session. Fix must propagate to all 6 call sites.

---

## Summary

- Total findings: 31 (P0: 0, P1: 5, P2: 13, P3: 13)
- Confidence: **medium-high** for P1 findings (each has a concrete code-path and reproduction story); **medium** for P2/P3 (some marked PLAUSIBLE because they need live testing to confirm).

**Most urgent fixes (P1):**
- `AUDIT-session-01` — SSH reconnect kills the keepalive goroutine on the same session instance.
- `AUDIT-session-03` — FTP `Disconnect` races with transfer goroutines; library-level panic risk.
- `AUDIT-session-04` — VNC/SPICE proxy stop leaks goroutines in the upgrade-race window.
- `AUDIT-session-05` — Telnet auto-login uses fixed sleeps; username can land in shell prompt.

---

## Phase 2 verification

**Date:** 2026-07-28
**Verifier:** gsd-verifier
**Per-finding verdict table:** `.planning/audit/phase-2/backend-session-verification.md`

### Verdict roll-up (31 findings)

| Verdict | Count | IDs |
|---------|-------|-----|
| CONFIRMED high | 1 | AUDIT-session-03 |
| CONFIRMED medium | 14 | 01, 04, 05, 07, 08, 09, 10, 11, 12, 13, 14, 15, 17, 19, 28 |
| CONFIRMED low | 11 | 16, 18, 21, 22, 24, 25, 27, 30, 31 + dupe count correction (see below) |
| PLAUSIBLE (deferred) | 5 | 01 (alt), 11, 14, 26, 29 |
| FALSE_POSITIVE / design | 4 | 02, 06, 23, 30 |

Note on counts: 01 appears in both CONFIRMED-medium and PLAUSIBLE because the
mechanism is real (single-use quit channel) but the current frontend flow does
not exercise it (`Panel.vue:402-420` always `CreateSession`s a fresh UUID on
reconnect). It stays in CONFIRMED medium because the SSHSession API itself is
unsafe to call `Connect` twice — any future caller would hit it.

### Top fix priorities (high then medium)

1. `AUDIT-session-03` — FTP Disconnect connMu (~5 lines)
2. `AUDIT-session-04` — VNC/SPICE Proxy stop leak window
3. `AUDIT-session-05` — Telnet auto-login prompt detection (replace sleeps)
4. `AUDIT-session-19` — `LocalSession` Unix Disconnect lifecycle symmetry
5. `AUDIT-session-13` — Mosh scanner buffer + Err check
6. `AUDIT-session-15` — FTP `ChangeRemoteDir` inside `connMu`
7. `AUDIT-session-28` — `output_log.go` CUF overflow cap in `parseCSIParam`
8. `AUDIT-session-17` — `ssh_auth.parsePrivateKeyFile` propagate errors
9. `AUDIT-session-12` — `tunnel_service.Shutdown` parallel teardown
10. `AUDIT-session-09` — `trimPostLoginOutput` UTF-8 boundary
11. `AUDIT-session-08` — `output_log.applyCSI` ICH `p.emitted` adjustment
12. `AUDIT-session-10` — `MonitorSession.pushSystemInfo` retry/backoff
13. `AUDIT-session-07` — Output log buffered writer (perf; not correctness)
14. `AUDIT-session-01` — SSHSession single-use clarification
15. `AUDIT-session-14` — SerialSession readLoop `<-s.quit` defensive select

### False positives / drop from queue

- `AUDIT-session-02` — `kbCallback` is invoked synchronously inside `ssh.NewClientConn`; the deferred `s.authAnswerCh = nil` fires after the handshake has already returned. The audit's race scenario requires the ssh library to invoke kbCallback asynchronously; it does not.
- `AUDIT-session-06` — `s.authAnswerCh` only carries frontend keystrokes (from `Write`); server banner arrives via `readLoop → emitData → xterm display` and never enters the auth channel.
- `AUDIT-session-23` — `s.bucket = ""` IS cleared at line 66; the audit acknowledges.
- `AUDIT-session-30` — `manager.Close` deliberately releases the lock before invoking blocking `Disconnect`; the audit agrees no fix is needed.