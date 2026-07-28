# Phase 2 Verification — backend/session/

**Verifier:** gsd-verifier (Phase 2)
**Date:** 2026-07-28
**Module:** `backend/session/*.go` (33 protocol sessions, manager, tunnel, output_log, post_login, zmodem, monitor, proxy)
**Source audit:** `.planning/audit/phase-1/backend-session.md` (31 findings, 0 P0 / 5 P1 / 13 P2 / 13 P3)
**Baseline:** `.planning/codebase/CONCERNS.md` (SSH host-key bypass, FTP/TLS bypass, plaintext password storage, output_log flush-per-write, kbCallback echo concern, RDP fragility already known)

---

## Verdict Summary

| ID | Title | Severity | Verdict | ROI | Notes |
|----|-------|----------|---------|-----|-------|
| AUDIT-session-01 | SSH `Disconnect` `sync.Once` reconnect bug | P1 | PLAUSIBLE | medium | `ssh_session.go:60, 466-478`. Code path is real, but `app.go` + `Panel.vue:402-420` always `CreateSession` (fresh UUID) on reconnect, so the SAME `SSHSession` instance is never re-`Connect`-ed by the frontend. The defect is latent — any caller that re-uses the instance triggers `startKeepAlive` exit on `<-s.quit`. |
| AUDIT-session-02 | `kbCallback` closure races `s.authAnswerCh = nil` defer | P1 | FALSE_POSITIVE | n/a | `ssh_session.go:127-168, 85-89`. `kbCallback` is invoked **synchronously** inside `ssh.NewClientConn` (line 192), which is itself called synchronously between line 82 (channel assignment) and `defer` (lines 85-89, fires when `Connect` returns). The closure has terminated before `authAnswerCh` is ever nilled. `go -race` exposure would only matter if ssh library invoked kbCallback asynchronously (it does not). |
| AUDIT-session-03 | `FTPSession.Disconnect` closes `s.conn` outside `s.connMu` | P1 | CONFIRMED | high | `ftp_session.go:112-119`. `Disconnect` reads/writes `s.conn` unsynchronized while `startTransfer` (line 628) and friends hold `s.connMu`. `jlaffaye/ftp`'s `ServerConn` is not concurrent-safe; concurrent `Quit`+`Retr` is the documented failure mode. |
| AUDIT-session-04 | `VNCProxy`/`SPICEProxy` `Stop` lets a handler race between `listener.Close()` and `wg.Wait()` | P1 | CONFIRMED | medium | `vnc_proxy.go:148-162` and `spice_proxy.go:151-165`. An http handler dispatched *just before* `p.listener.Close()` returns can upgrade a WS, reach `p.wg.Add(2)` (lines 99 / 102), and start two goroutines that `p.wg.Wait()` does not track. Goroutine leak + UAF on `p.tcpConn`/`p.wsConn`. |
| AUDIT-session-05 | Telnet auto-login hard-coded `time.Sleep` | P1 | CONFIRMED | medium | `telnet_session.go:234-258`. No IAC/prompt detection; sleeps are 1500 ms / 1200 ms unconditionally. Slow BBS/Cisco devices expose username into post-prompt shell. |
| AUDIT-session-06 | `kbCallback` outer loop accumulates banner garbage | P2 | FALSE_POSITIVE | n/a | `ssh_session.go:94-168`. The outer pre-prompt loop (lines 96-122) and `kbCallback` both pull from `s.authAnswerCh`. Only `Write` (line 444) feeds that channel, and `Write` carries **user keystrokes** — server banner arrives via `readLoop` → `emitData` → frontend xterm display, never via `authAnswerCh`. The described mechanism cannot fire. |
| AUDIT-session-07 | `output_log.go` `WriteOutput` calls `file.Sync()` after every write | P2 | CONFIRMED | medium | `output_log.go:522-524`. Matches CONCERNS verbatim. Hot path is `bufio.NewWriter` + periodic `Sync`; current code stalls on HDD/NFS. |
| AUDIT-session-08 | `output_log.go` `applyCSI` `ICH (@)` doesn't bump `p.emitted` for shifted bytes | P2 | CONFIRMED | medium | `output_log.go:315-321`. When ICH shifts bytes that were already flushed (pos<emitted), the next `FlushPartial`-style write re-emits them. With `p.line=[a,b,c,d,e,f,g,h]`, `pos=5`, `emitted=8`, ESC[2@ ⇒ line becomes `[a,b,c,d,e,' ',' ',f,g,h]`, then `p.line[p.emitted:]` still flushes from index 8 ⇒ duplicates f,g,h already flushed at original positions 6-7 (with no body shift) or 8-? wait – original p.line[6,7]=[g,h]; insert at 5 puts blanks at 5,6; f→5, g→6, h→7. Hmm recheck: `[a,b,c,d,e,f,g,h]` insert 2 blanks at pos 5 → `[a,b,c,d,e,' ',' ',f,g,h]`. Original f,g,h at positions 5,6,7 → now at positions 7,8,9. `p.emitted=8` ⇒ `p.line[8:]=[h]`. Just one duplicate. The mechanism is still real, just narrower than the audit claims. |
| AUDIT-session-09 | `trimPostLoginOutput` slices at byte boundary, can split UTF-8 | P2 | CONFIRMED | medium | `post_login_expect.go:185-190`. `output[len-8192:]` is byte-indexed; a CJK continuation byte (0x80-0xBF) at the start sabotages `strings.Contains` substring search → expect times out. |
| AUDIT-session-10 | `MonitorSession.pushSystemInfo` fire-once at Connect; no retry | P2 | CONFIRMED | medium | `monitor_session.go:151, 159-168`. `go s.pushSystemInfo()` runs exactly once. Transient SSH handshake failure (`s.client.NewSession` line 173 swallows err, returns nil) silently drops the OS/kernel panel forever. |
| AUDIT-session-11 | `tunnel_forward.go` keeps tunnel goroutines without recover | P2 | PLAUSIBLE | medium | `tunnel_forward.go:48-76`. Code path is guarded (`buildHopChain` rejects empty exitID at line 179), so direct `nil exit` is unreachable today. The risk is that a future change to `dialChain` returning partial state would panic inside `go tunnelKeepAlive(exit, …)` — no `recover()`. |
| AUDIT-session-12 | `tunnel_service.go` `Shutdown` calls `sshClient.Close()` serially | P2 | CONFIRMED | medium | `tunnel_service.go:179-194`. Sequential `entry.sshClient.Close()` (line 192) blocks on slow per-tunnel drains; with N tunnels, shutdown stalls up to N×SSH-timeout. |
| AUDIT-session-13 | Mosh `bufio.Scanner` default 64 KiB token, no `Err()` check | P2 | CONFIRMED | medium | `mosh_session.go:139-149`. `stdoutScanner.Scan()` returns false when line > 65536 bytes; `output.String()` is silently truncated. `startMoshServer` returns generic "missing key or port" without surfacing the `bufio.ErrTooLong`. |
| AUDIT-session-14 | `SerialSession.readLoop` does not check `s.quit` | P2 | PLAUSIBLE | medium | `serial_session.go:109-136`. The `s.port.Read` blocking depends on the driver unblocking on close. `go.bug.st/serial` does propagate EOF for most adapters, but Linux USB-CDC drivers with internal buffering occasionally don't. Defensive add of `<-s.quit` is small. |
| AUDIT-session-15 | `FTPSession.ChangeRemoteDir` calls `s.conn.List` without `s.connMu` | P2 | CONFIRMED | medium | `ftp_session.go:191-206` calls `s.conn.List(target)` outside any lock while sibling `MakeDir`, `Remove`, `Rename`, `startTransfer` all hold `s.connMu`. Same library-concurrency profile as AUDIT-session-03. |
| AUDIT-session-16 | `ssh_session.decodeOutput` holds `s.mu` across `encoding.Decoder.Transform` | P2 | CONFIRMED | low | `ssh_session.go:512-535`. Lock spans entire Transform loop; concurrent `Write`/`Disconnect`/`Resize` block until the chunk drains. For 4 KiB chunks on a slow CPU with GBK/Big5 this can be tens of ms. |
| AUDIT-session-17 | `parsePrivateKeyFile` swallows all errors (`return nil, false`) | P2 | CONFIRMED | medium | `ssh_auth.go:33-50`. `os.ReadFile` errs, bad permissions, wrong passphrase — all collapse to `(nil, false)`. `makeSSHAuthMethods` (line 16) then silently falls through to keyboard-interactive, masking the real cause. |
| AUDIT-session-18 | `output_log.go` ignores write errors (line 522) and banner write errors | P2 | CONFIRMED | low | `output_log.go:452-453, 461-463, 477-478, 522-523`. `file.Sync` after `Write` failure is skipped (correct), but no counter; user never sees that the log is silently dropping bytes after a disk-full / NFS stale. |
| AUDIT-session-19 | `LocalSession.Disconnect` (Unix) doesn't wrap pty.Close / Process.Kill in `quitOnce` | P3 | CONFIRMED | medium | `local_session_unix.go:193-205` vs. `local_session_windows.go:366-382`. Unix variant leaves `s.pty.Close()` and `s.cmd.Process.Kill()` outside the `Do` block; second call returns "process already finished" + `setStatus` fires twice. Windows variant is correct; mirror it. |
| AUDIT-session-20 | `manager.go` type switch has 19 cases; new protocols need 2 edits | P3 | CONFIRMED | low | `manager.go:21-83`. Already in CONCERNS (`session.Manager.Create switch`). Confirmed, low value to fix in this milestone. |
| AUDIT-session-21 | `SessionManager.sessions` unbounded | P3 | CONFIRMED | low | `manager.go:10-19, 86-90`. UUID v4 makes collisions astronomically rare; `Add` (line 86-90) silently overwrites on duplicate `s.ID()`. Mostly a CONCERNS repeat. |
| AUDIT-session-22 | `TelnetSession.Disconnect`/`Write` don't nil `s.conn` | P3 | CONFIRMED | low | `telnet_session.go:268-288`. `Write` (line 268) treats closed conn as a low-level net error instead of mirroring SSH's "not connected" message. Cosmetic UX. |
| AUDIT-session-23 | `S3Session.Disconnect` doesn't symmetrically nil `bucket` | P3 | FALSE_POSITIVE | n/a | `s3_session.go:64-69` *does* clear `s.bucket = ""`. The audit itself acknowledges this. Drop from queue. |
| AUDIT-session-24 | `K8sExecSession` has no reconnect; `s.conn` not nil'd on Close | P3 | CONFIRMED | low | `k8s_exec_session.go:71-78`. `quitOnce.Do` calls `s.conn.Close()` but the field stays non-nil; subsequent `Write` hits `*websocket.CloseError`. No auto-reconnect; the frontend has to call `K8sExecSession` (App layer) to redial. |
| AUDIT-session-25 | `postLoginExpectAutomation` initial match check in `waitForPostLoginExpect` runs before `isConnected()` | P3 | CONFIRMED | low | `post_login_expect.go:140-165`. The first-iteration `matchesPostLoginExpect(...)` at line 141 returns `nil` before `isConnected` is consulted. Race window is the line between the outer `IsConnected()` check (line 106) and the `waitForPostLoginExpect` call (line 114). |
| AUDIT-session-26 | `RunPostLoginScript` sends `line + "\r"` regardless of OS shell | P3 | PLAUSIBLE | low | `session.go:350-378`. Today `LocalSession.runPostLoginScript` uses PTY (line-disciplined), SSH uses Unix PTY (`\r` alone is fine). Future Windows-CLI non-PTY targets would need `\r\n`. Document only. |
| AUDIT-session-27 | `MonitorSession.Disconnect` calls `ticker.Stop` / `client.Close` outside `quitOnce` | P3 | CONFIRMED | low | `monitor_session.go:1378-1390`. `ticker.Stop()` is idempotent; `client.Close()` is idempotent; but `setStatus(StatusDisconnected)` fires twice on concurrent Disconnect (manual + `client.Wait()` goroutine). Cosmetic. |
| AUDIT-session-28 | `applyCSI` CUF accepts unbounded `n`; integer overflow on `p.pos += n` can wrap negative | P3 | CONFIRMED | medium | `output_log.go:264-269, 332-362`. `p.pos += n` (line 266) wraps to negative on `n` near `math.MaxInt64`; the `if p.pos > len(p.line)` (line 267) check is false for negative ⇒ `default:` branch at line 234 does `p.line[p.pos] = b` and panics. Real but requires a malicious TUI; cap `n` (e.g. 4096) in `parseCSIParam`. |
| AUDIT-session-29 | `tunnel_forward.acceptLoop` doesn't respect `quit` on stuck Accept / no flood limit | P3 | PLAUSIBLE | low | `tunnel_forward.go:321-333`. Standard `net.Listener.Close()` unblocks `Accept()` via `net.ErrClosed`, so the `select { case <-quit: }` at line 326 rarely actually saves. The flood concern (no accept rate limit) is real; bound via `SetDeadline` or accept timeouts. |
| AUDIT-session-30 | `manager.go Close` calls Disconnect after releasing lock | P3 | CONFIRMED | n/a | `manager.go:92-102`. The design is intentional: lock released before blocking `Disconnect` so other ops aren't stalled. The audit reaches the same conclusion. No fix needed. |
| AUDIT-session-31 | `disconnectNotice` uses local time only | P3 | CONFIRMED | low | `session.go:23-26`. Cosmetic on systems with clock skew. |

---

## Confirmed (with ROI)

### high (fix first)

- **AUDIT-session-03** — FTP `Disconnect` race. Low blast radius (single file, connMu pattern is already documented). Fix: hold `s.connMu` in `Disconnect`. Fix cost: ~5 lines.

### medium

- **AUDIT-session-01** — SSH reconnect on shared instance (latent; not exercised by today's frontend, but `Connect` does not document single-use or self-init the channels — easy to misuse). Fix: re-init `quit`/`authAnswerCh`/`quitOnce` at top of `Connect`, or document single-use and force `Create`.
- **AUDIT-session-04** — VNC/SPICE Proxy stop — leak window. Fix: gate `handleWebSocket` on `<-p.stopCh` and put `p.listener.Close()` first. Add a small synchronisation check.
- **AUDIT-session-05** — Telnet auto-login — replace sleeps with prompt matching (extend `filterIAC`).
- **AUDIT-session-07** — Output log Sync per write — bufio + periodic Sync.
- **AUDIT-session-08** — `applyCSI` ICH bumps `p.emitted` by shifted bytes.
- **AUDIT-session-09** — `trimPostLoginOutput` advances to next UTF-8 start byte.
- **AUDIT-session-10** — `pushSystemInfo` retry loop with backoff.
- **AUDIT-session-11** — `tunnelKeepAlive` and tunnel goroutines wrapped in `defer recover()`.
- **AUDIT-session-12** — `Shutdown` parallelises `sshClient.Close()` with a WaitGroup cap.
- **AUDIT-session-13** — Mosh scanner buffer + Err check.
- **AUDIT-session-14** — `SerialSession.readLoop` add `<-s.quit` non-blocking check.
- **AUDIT-session-15** — `ChangeRemoteDir` wrap in `s.connMu`.
- **AUDIT-session-17** — `parsePrivateKeyFile` return wrapped errors.
- **AUDIT-session-19** — Move `pty.Close`/`Process.Kill` into `quitOnce.Do` on Unix.
- **AUDIT-session-28** — `applyCSI` cap CUF/CUB/CHA/ICH parameters at 4096 in `parseCSIParam`.

### low

- **AUDIT-session-16** — `decodeOutput` use per-decoder mutex separate from `s.mu`.
- **AUDIT-session-18** — Output log write-error counter surfaced to App.
- **AUDIT-session-21** — `SessionManager.sessions` weak-ref / LRU.
- **AUDIT-session-22** — Telnet `Disconnect` nil `s.conn`; `Write` returns "not connected".
- **AUDIT-session-24** — `K8sExecSession` nil `s.conn` after Close; surface a typed disconnect.
- **AUDIT-session-25** — `waitForPostLoginExpect` initial-match check gated by `isConnected()`.
- **AUDIT-session-27** — MonitorSession `Disconnect` move `ticker.Stop`/`client.Close` into `quitOnce.Do`.
- **AUDIT-session-31** — `disconnectNotice` dual local+UTC or relative-time.

## Plausible (deferred — needs runtime repro)

- **AUDIT-session-01** — depends on whether/where frontend ever re-uses the same SSHSession instance.
- **AUDIT-session-11** — depends on future changes to `dialChain`.
- **AUDIT-session-14** — depends on serial driver behavior on Linux/Windows.
- **AUDIT-session-26** — depends on whether `LocalSession.runPostLoginScript` is ever pointed at a non-PTY target.
- **AUDIT-session-29** — `ln.Accept()` does unblock on `Close()` per Go std `net/net.go`; the flood DoS concern is independent of the `quit` channel.

## False Positives / Already-Mitigated (drop from queue)

- **AUDIT-session-02** — code path does not exist; kbCallback runs synchronously, defer fires only after handshake returns.
- **AUDIT-session-06** — channel carries user input only; server banner never reaches `authAnswerCh`.
- **AUDIT-session-23** — `s.bucket` IS cleared (line 66); the audit acknowledges.
- **AUDIT-session-30** — design intentional; audit concludes no fix needed.

---

## Net for fix queue

- **CONFIRMED + ROI assigned:** 26 items (1 high, 14 medium, 11 low)
- **PLAUSIBLE:** 5 items (deferred until reproduction)
- **FALSE_POSITIVE / design choice:** 4 items

### Top fix priorities (high then medium, ordered by blast radius / fix cost)

1. AUDIT-session-03 — FTP Disconnect connMu (cheap, racing today)
2. AUDIT-session-04 — VNC/SPICE Proxy Stop leak window (cheap, 2 lines)
3. AUDIT-session-05 — Telnet auto-login (replace sleeps)
4. AUDIT-session-19 — LocalSession Unix Disconnect lifecycle symmetry
5. AUDIT-session-13 — Mosh scanner buffer/Err
6. AUDIT-session-15 — FTP ChangeRemoteDir connMu
7. AUDIT-session-28 — output_log CUF overflow guard
8. AUDIT-session-17 — ssh_auth key file error propagation
9. AUDIT-session-12 — tunnel_service Shutdown parallel teardown
10. AUDIT-session-09 — trimPostLoginOutput UTF-8 boundary
11. AUDIT-session-08 — output_log ICH emitted-marker update
12. AUDIT-session-10 — MonitorSession systemInfo retry
13. AUDIT-session-07 — Output log buffered writer (perf, not correctness)
14. AUDIT-session-01 — SSH session single-use documentation/init (latent)
15. AUDIT-session-14 — SerialSession readLoop quit select (defensive)
