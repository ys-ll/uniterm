# Phase 2 Verification — backend/k8s/

**Verifier:** Claude (Phase 2 verification subagent)
**Date:** 2026-07-28
**Phase 1 source:** `.planning/audit/phase-1/backend-k8s.md` (14 findings: 0 P0 / 2 P1 / 5 P2 / 7 P3)
**Baseline:** `.planning/codebase/CONCERNS.md` (TLS bypass @ client.go:65; authRoundTripper no Closer @ client.go:133-144)

**Method:** Read cited file:line + surrounding context; construct failure scenario; classify CONFIRMED / PLAUSIBLE / FALSE_POSITIVE. CONFIRMED items get ROI (high / medium / low).

---

## Verdict Summary

| ID | Title | Severity | Verdict | ROI | Notes |
|----|-------|----------|---------|-----|-------|
| K8S-01 (P1-1) | authRoundTripper no 401 retry | P1 | CONFIRMED | high | `client.go:138-144` RoundTrip only injects header, never inspects `resp.StatusCode`; exec-provider users with rotating tokens get permanent 401s after token TTL expires |
| K8S-02 (P1-2) | Watch / log streams never reconnect after apiserver bounce | P1 | CONFIRMED | medium | `watch.go:48-60` / `logs.go:37-41` loop calls `onEnd(scanner.Err())` once and exits; `manager.go:190-203, 253-262` onEnd only deletes map entry and emits `k8s:watch-end:<id>` / `k8s:log-end:<id>` — no reconnect, no backoff (grep finds zero `time.Sleep`/`backoff`/`reconnect` in module) |
| K8S-03 (P2-1) | http.Client has no MaxIdleConns / IdleConnTimeout cap | P2 | CONFIRMED | low | `client.go:50-53` constructs bare `&http.Transport{TLSClientConfig, Proxy}` — no `MaxIdleConns` / `MaxIdleConnsPerHost` / `IdleConnTimeout`; default stdlib pool is per-client |
| K8S-04 (P2-2) | DialExec does not accept context.Context | P2 | CONFIRMED | medium | `manager.go:288-335` signature is `(connID, ns, pod, container, cols, rows int)`; `dialer.Dial` at line 330 is not cancellable; tab close / `Disconnect` cannot unblock the pending handshake |
| K8S-05 (P2-3) | rest.Do has unbounded io.ReadAll | P2 | CONFIRMED | medium | `rest.go:38` `b, err := io.ReadAll(resp.Body)` reads whole body into memory before returning; large `pods -A` (5–10k pods, multi-MB JSON) balloons RSS on every call |
| K8S-06 (P2-4) | EventEmitter called synchronously, no back-pressure | P2 | CONFIRMED | low | `manager.go:189` and `manager.go:252` call `emit(eventName, ...)` on the watch/log goroutine itself; Wails `EventsEmit` is documented best-effort — blocked channel stalls the goroutine, which in turn stalls the apiserver TCP read (compounds K8S-02) |
| K8S-07 (P2-5) | exec provider parsed but never invoked | P2 | CONFIRMED | high | `kubeconfig.go:149-158` populates `user.Exec`; `client.go:58` ignores it and only uses `user.Token`; every exec-only kubeconfig (GKE / EKS / AKS) gets an empty bearer and the very first call returns 401 — same fix as K8S-01 |
| K8S-08 (P2-6) | Strict base64 rejects valid k8s-encoded data | P2 | CONFIRMED | low | `kubeconfig.go:121, 137, 143` use `base64.StdEncoding.DecodeString`; `kubectl` writes single-line base64 but `yq`-exported / hand-edited configs wrap to 64 cols with embedded `\n` — one broken entry fails the whole `ParseBytes` (no per-cluster isolation) |
| K8S-09 (P2-7) | authRoundTripper mutates caller's req.Header in-place | P2 | CONFIRMED | medium | `client.go:139-140` calls `req.Header.Set("Authorization", ...)`; the standard `http.Client` redirect chain, dump-request debug layers, and any future retry path will see an already-set `Authorization` and skip re-injection — directly blocks K8S-01 fix |
| K8S-10 (P3-1) | No Manager.Close / shutdown method | P3 | CONFIRMED | low | `manager.go` has no `Close()` / `Shutdown(ctx)`; `main.go:103` wires `app.shutdown` to `OnShutdown`; `app.go` shutdown at line 269 only calls `sessionManager.CloseAll()` — `k8sManager` is left for the OS to reap |
| K8S-11 (P3-2) | rest.Do no 4xx/5xx error classification | P3 | CONFIRMED | low | `rest.go:16-49` returns `(status, body, nil)` for every status code; callers (frontend / app.go:4059) cannot distinguish "client error" from "server transient" from "success" without re-inspecting `status` |
| K8S-12 (P3-3) | buildLogPath accepts negative tailLines / empty container | P3 | CONFIRMED | low | `logs.go:46-51` formats with `%d` blindly; `tailLines=-1` or `container=""` produces a syntactically valid URL that apiserver rejects with `400 Bad Request` instead of a clean Go error |
| K8S-13 (P3-4) | Captured emit reference ignores later SetEventEmitter swaps | P3 | CONFIRMED | low | `manager.go:183` (`emit := m.emit`) and `manager.go:246` (`client, base, emit := conn.client, conn.base, m.emit`) capture-by-value at watch/log start; `SetEventEmitter` swaps after stream start are invisible |
| K8S-14 (P3-5) | kubeconfig.go silently drops malformed exec.env entries | P3 | CONFIRMED | low | `kubeconfig.go:151-153` builds `env[kv["name"]] = kv["value"]` with no check that keys are non-empty; `env: - foo` produces `{"":""}` which collides with reserved vars the exec plugin relies on |
| K8S-15 (P3-6) | Watch stream bound by scanner.Buffer 4 MiB per line | P3 | CONFIRMED | low | `watch.go:47` and `logs.go:36` both `scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)`; CRD / ConfigMap annotation >4 MiB returns `bufio.ErrTooLong`, the stream ends |
| K8S-16 (P3-7) | base + path joined via raw string concat | P3 | CONFIRMED | low | `rest.go:24` `base+path`; `watch.go:27` `base+path`; `logs.go:19` `base+path`; trailing-slash on `cluster.Server` (hand-edited / ingress URL) produces `//api/v1/...` — apiserver tolerates but reverse proxies may 404 |
| K8S-17 (P3-8) | ParseServerAddr hard-codes IPv4 loopback in LocalAddr | P3 | CONFIRMED | low | `server_addr.go:42-44` `net.JoinHostPort("127.0.0.1", strconv.Itoa(port))` ignores the cluster URL family; IPv6 cluster `[fd00::1]:6443` with SSH tunnel still gets an IPv4 local bind (works by accident because SSH tunnel listens on `0.0.0.0`, but the abstraction lies) |

---

## Confirmed (with ROI)

**high** (2):
- **K8S-01** — `authRoundTripper.RoundTrip` (client.go:138-144) injects the bearer header but never reads `resp.StatusCode`. After exec-plugin token rotation (every ~15 min on GKE/EKS/AKS) the first call returns 401 and every subsequent `Manager.Request` / `StartWatch` / `StartLogStream` / `DialExec` propagates the same 401 verbatim. Only `Disconnect → Connect` restores access. High because every managed-K8s user is affected.
- **K8S-07** — `BuildClientWithDial` (client.go:58) wires `token: user.Token` and discards `user.Exec`. Every kubeconfig whose auth is `exec:` only (no `token:` field — the dominant pattern for GKE / EKS / AKS / Rancher) has empty bearer; first request is 401. Same fix as K8S-01 — both collapse into one exec-provider implementation in `authRoundTripper`.

**medium** (4):
- **K8S-02** — watch goroutine (watch.go:48-60) and log goroutine (logs.go:37-41) call `onEnd(scanner.Err())` once and exit; onEnd closures (manager.go:190-203, 253-262) delete from map and emit `k8s:watch-end:<id>` / `k8s:log-end:<id>` — no reconnect loop anywhere (grep confirms zero `time.Sleep` / `backoff` / `reconnect` in the module). User must manually restart every watch on apiserver bounce. Compounds with K8S-06 (sync emit blocks scanner).
- **K8S-04** — `DialExec` (manager.go:288-335) signature is `(connID, ns, pod, container string, cols, rows int)` — no `context.Context`. `dialer.Dial` (line 330) is uncancellable; `Disconnect` cannot abort the pending handshake, so the UI's "cancel" button is a lie until TCP timeout.
- **K8S-05** — `Do` (rest.go:38) reads `resp.Body` via `io.ReadAll` with no `http.MaxBytesReader` / `LimitReader`. `pods -A` over a 5k-pod cluster returns multi-MB JSON that is buffered entirely into the Wails process; repeated calls (e.g. polling timer) wedge the UI.
- **K8S-09** — `RoundTrip` (client.go:139-140) calls `req.Header.Set("Authorization", "Bearer "+a.token)` — this mutates the caller's request in place. Any future retry path that re-uses `*http.Request` will see the header already set and skip re-injection. Also affects redirect-following chains (3xx) where the same Request is reused. Blocks the K8S-01 fix unless `req.Clone(req.Context())` is added first.

**low** (11):
- **K8S-03** — `&http.Transport{TLSClientConfig, Proxy}` (client.go:50-53) lacks `MaxIdleConns` / `MaxIdleConnsPerHost` / `IdleConnTimeout`; stdlib defaults (100 / 2 / none) accumulate idle conns across many clusters. Pairs with the existing CONCERNS "authRoundTripper missing-idle-close" finding.
- **K8S-06** — `emit(eventName, ev)` (manager.go:189) and `emit(eventName, line)` (manager.go:252) run synchronously on the scanner goroutine; Wails `EventsEmit` is best-effort. Event burst stalls the goroutine → apiserver TCP read pauses → socket times out → K8S-02.
- **K8S-08** — `base64.StdEncoding.DecodeString` (kubeconfig.go:121, 137, 143) rejects whitespace; a single broken `certificate-authority-data` (wrapped to 64 cols by `yq` / hand-edit) fails the entire `ParseBytes` (line 123 wraps as `cluster %q CA data: %w`). No per-cluster isolation.
- **K8S-10** — `Manager` has no `Close()` / `Shutdown(ctx)`; `main.go:103` wires `OnShutdown` to `app.shutdown`; `app.go` shutdown (line 269) only calls `sessionManager.CloseAll()`. `k8sManager`'s watches / log streams / idle conns are reaped only by the OS process exit.
- **K8S-11** — `Do` returns `(status, body, nil)` for every status (rest.go:32-49); callers cannot branch on error type without re-inspecting status.
- **K8S-12** — `buildLogPath` (logs.go:46-51) formats `tailLines=%d` blindly; `tailLines=-1` or `container=""` produces valid-looking URL that apiserver 400s.
- **K8S-13** — `emit := m.emit` (manager.go:183) and `client, base, emit := conn.client, conn.base, m.emit` (manager.go:246) capture-by-value; later `SetEventEmitter` swaps are invisible to in-flight streams.
- **K8S-14** — `env[kv["name"]] = kv["value"]` (kubeconfig.go:151-153) with no `name != ""` check; `env: - foo` silently becomes `{"":""}`.
- **K8S-15** — `scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)` (watch.go:47, logs.go:36); CRD annotation >4 MiB → `bufio.ErrTooLong` → stream ends.
- **K8S-16** — `base+path` (rest.go:24, watch.go:27, logs.go:19) — no trailing-slash normalization.
- **K8S-17** — `LocalAddr(port)` (server_addr.go:42-44) hard-codes `"127.0.0.1"`; ignores IPv6 cluster URLs.

---

## Plausible (deferred)

_None._ All 14 phase-1 findings map to code that materially exists; no scenarios required deferred verification.

---

## False Positives (drop)

_None._ No phase-1 finding was found to be inaccurate against the source.

---

## Net for fix queue

**CONFIRMED + (high|medium): 6** — 2 high (K8S-01, K8S-07), 4 medium (K8S-02, K8S-04, K8S-05, K8S-09).

**Recommended bundling:**
1. **One patch:** K8S-01 + K8S-07 + K8S-09 — implement exec provider in `authRoundTripper` with `req.Clone` and one-shot 401 retry.
2. **One patch:** K8S-02 + K8S-06 — add bounded backoff + reconnect inside the watch/log goroutines, plus bounded buffered channel between scanner and emitter.
3. **One patch:** K8S-04 — `DialExec` signature change is a breaking API; coordinate with `app.go:4087` callers.
4. **One patch:** K8S-05 — wrap `resp.Body` in `http.MaxBytesReader` (16 MiB cap) in `rest.Do`.
5. **Batch the P3s** into a single cleanup PR; no runtime risk, just hygiene.

---

## Summary

Verified 14. CONFIRMED=14 (high=2, medium=4, low=8), PLAUSIBLE=0, FALSE_POSITIVE=0. Top fix: K8S-01 + K8S-07 + K8S-09 (exec-auth + 401 retry + req.Clone — bundled); K8S-02 + K8S-06 (watch/log reconnect + back-pressure — bundled); K8S-04 (DialExec context); K8S-05 (rest.Do size cap).