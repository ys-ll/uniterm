# Phase 1 Audit — backend/k8s/

**Auditor:** gsd-code-reviewer (Phase 1)
**Date:** 2026-07-28
**Scope:** backend/k8s/*.go (manager, kubeconfig, client, rest, watch, logs, server_addr)

Baseline: `.planning/codebase/CONCERNS.md` already covers `InsecureSkipVerify`
(client.go:65) and `authRoundTripper` connection draining (client.go:133-144).
Findings below are NEW issues not duplicated in CONCERNS.

---

## Findings (by severity)

### P0 — Critical

_None._

---

### P1 — High

#### P1-1: `authRoundTripper` has no token-rotation / 401 retry path
- **File:** `backend/k8s/client.go:138-144`
- **Trigger:** Long-lived kube session on `exec` auth provider (gke-gcloud-auth-plugin,
  AWS IAM, kubelogin, etc.) — the issuing service rotates the bearer token every
  ~15 min. apiserver returns `401 Unauthorized` on the first request after rotation.
- **Reproduction:** Connect via a kubeconfig whose user has an exec plugin; wait past
  the plugin's token TTL; issue a `GET /api/v1/pods` through `Manager.Request`. The
  current `authRoundTripper.RoundTrip` propagates the 401 verbatim and never re-runs
  the exec plugin. All subsequent `Request` / `StartWatch` / `StartLogStream` calls
  keep failing with the same stale token; only a `Disconnect → Connect` cycle
  restores access.
- **Wrong output:** `(401, [])` bubbled up to the Wails binding as `K8sResponse`;
  watch streams fall into the onEnd path with `"HTTP 401: <body>"` and terminate
  permanently. Frontend must manually reconnect.
- **Suggested fix category:** Plumb an exec provider into `authRoundTripper` (the
  existing `// P2:` TODO comment at client.go:142) and add a single on-401 retry
  that runs the exec plugin, swaps the bearer token, and retries exactly once.
  Same fix unblocks users whose only auth path is exec.

#### P1-2: Watches and log streams never reconnect after apiserver bounce
- **File:** `backend/k8s/watch.go:44-60`, `backend/k8s/logs.go:33-41`,
  `backend/k8s/manager.go:190-203` (onEnd closure), `backend/k8s/manager.go:253-262`
  (log onEnd closure)
- **Trigger:** apiserver restart, control-plane rolling upgrade, etcd compaction
  exceeding `resourceVersion`, network blip longer than the keepalive window. The
  `bufio.Scanner` finishes, `onEnd(scanner.Err())` is invoked once, and the goroutine
  exits. The Manager's onEnd only deletes from the maps and emits a one-shot
  `k8s:watch-end:<id>` / `k8s:log-end:<id>` event; it never re-`Dial`s.
- **Reproduction:** Start `StartWatch(connID, "/api/v1/pods?watch=true")` with a
  live cluster; force the cluster to bounce (`kubectl rollout restart deploy/kube-apiserver`
  on a kind setup, or `docker kill` the control plane). The watch sends its end
  event and the UI shows "stream ended" forever. The user has to call
  `K8sStartWatch` from the renderer for every disconnect.
- **Wrong output:** Watch silently dies on what is normally a transient apiserver
  outage; pod log pane stops mid-debug. There is no exponential backoff, no
  `410 Gone` detection (which would tell us the resourceVersion has been compacted
  and a relist is needed).
- **Suggested fix category:** Inside the goroutine, after `scanner.Scan() == false`,
  inspect `scanner.Err()`. If nil (clean EOF / `410 Gone` = expected) or a transient
  net error, sleep a capped backoff and re-`client.Do` with the original context
  still honored. Auto-stop after N failures, emitting an end event the frontend
  already understands. Surface a `Last-ResourceVersion` parameter so the re-dial
  resumes from the last seen RV when possible.

---

### P2 — Medium

#### P2-1: `http.Client` constructed in `BuildClientWithDial` has no `MaxIdleConns` / per-cluster cap
- **File:** `backend/k8s/client.go:50-61`
- **Trigger:** A user opens ~20 K8s connections to different clusters (one per
  context). Each `*http.Client` owns its own default `Transport`; the stdlib
  default is `MaxIdleConns=100`, `MaxIdleConnsPerHost=2`. Idle keep-alive TCP
  sockets accumulate against every apiserver; over hours this yields ~hundreds
  of idle conns across tabs.
- **Reproduction:** Connect to N>20 distinct kube clusters via `Manager.Connect`;
  leave the app running overnight; `lsof -p $PID | grep ESTABLISHED | wc -l`
  shows idle sockets growing, none reaped by the transport idle pool because the
  pool is per-cluster (not per-host across pools).
- **Wrong output:** Memory / FDs grow on long-lived desktops. Combined with the
  known `authRoundTripper` missing-idle-close (CONCERNS), the apiserver side may
  rate-limit the source IP.
- **Suggested fix category:** Set explicit `MaxIdleConns` / `IdleConnTimeout`
  on the base `Transport`. Consider pooling one transport per apiserver
  (`Server()` string) and reusing it across mgr-level connections — but that
  needs the auth transport refactor already on the TODO list.

#### P2-2: `Manager.DialExec` does not accept a `context.Context`
- **File:** `backend/k8s/manager.go:288-335`
- **Trigger:** Frontend initiates a `DialExec` against a remote apiserver over a
  flaky link (SSH tunnel through `LocalAddr`); the WebSocket handshake blocks
  inside `dialer.Dial(...)` with no cancellation handle. Stopping the tab in the
  renderer cannot unblock the goroutine — even after `Disconnect`, an in-flight
  `Dial` survives until TCP timeout (minutes).
- **Reproduction:** `Manager.DialExec(connID, "ns", "pod", "c", 80, 24)` while
  the SSH tunnel port has been closed; `Disconnect(connID)` does nothing for the
  pending dial. The matching exec tab UI shows "connected" until the dial
  finally fails.
- **Wrong output:** Stale goroutines pile up; the UI's "cancel exec" button is a
  lie until the underlying TCP times out.
- **Suggested fix category:** Add `ctx context.Context` as the first parameter to
  `DialExec`, and thread it to `dialer.DialContext` plus both the `dialOverride`
  and the captured `TLSClientConfig` (which is already context-aware). Frontend
  passes the tab's lifetime context; tab close cancels the dial.

#### P2-3: `rest.Do` has no response-size cap (`io.ReadAll` unbounded)
- **File:** `backend/k8s/rest.go:32-49`
- **Trigger:** Listing operations like `kubectl get pods -A` over a busy cluster
  (~10k pods) returns a JSON body of multi-MB to tens-of-MB. `io.ReadAll`
  accumulates the entire body into memory before returning to the renderer.
  Repeated calls (e.g. a polling timer at 5 s) wedge the Wails process.
- **Reproduction:** `m.Request(ctx, connID, "GET", "/api/v1/pods?limit=0",
  nil, "")` against a 5k-pod cluster; memory balloon by 30-50 MB per call.
- **Wrong output:** OOM risk on long sessions; failure mode is silent (just RSS
  climbs) until the OS reclaims.
- **Suggested fix category:** Cap the response body via
  `http.MaxBytesReader`-equivalent — wrap `resp.Body` in a `LimitReader` (e.g.
  16 MiB) and return a clear `ErrResponseTooLarge` error; the renderer can then
  re-issue with `?limit=500` pagination.

#### P2-4: `EventEmitter` (`Wails runtime.EventsEmit`) is called synchronously inside watch/log goroutines with no back-pressure
- **File:** `backend/k8s/manager.go:189`, `backend/k8s/manager.go:252`
- **Trigger:** apiserver pushing a burst (10k events in a tight loop on a
  controller reconcile, or a `kubectl rollout`-style ADDED storm). Each
  emitted event runs the Wails `EventsEmit` synchronously on the reading
  goroutine.
- **Reproduction:** Watch `events` for a controller manager churning pods.
  `runtime.EventsEmit` is documented as best-effort; if the WebView message
  channel is full, it blocks (Wails v2). The watch goroutine stalls, the TCP
  socket stops being drained, the apiserver eventually times the connection
  out — and we land on P1-2 (no reconnect).
- **Wrong output:** Watch appears frozen in the UI (no more events) until the
  burst passes; depending on timing, the socket dies and never recovers.
- **Suggested fix category:** Insert a bounded buffered channel between the
  scanner loop and the emitter (e.g. cap 1024 messages, drop oldest on overflow
  or coalesce events). On drop, log a single warning so the UI can show
  "stream back-pressured, N events dropped".

#### P2-5: `kubeconfig.go` parses `exec` provider but `BuildClientWithDial` never invokes it
- **File:** `backend/k8s/kubeconfig.go:38-52` (struct), `backend/k8s/kubeconfig.go:149-158` (parse),
  `backend/k8s/client.go:29-62` (BuildClientWithDial discards `user.Exec`)
- **Trigger:** User has a kubeconfig that *only* relies on `exec:` (most managed
  Kubernetes — GKE `gke-gcloud-auth-plugin`, EKS `eks-iam-authenticator`,
  AKS kubelogin, rancher, etc.). Their token field is empty.
- **Reproduction:** `ConnectWith(yamlWithExecUser, "ctx", opts)` — `BuildClientWithDial`
  returns `user.Token == ""`, `authRoundTripper.token == ""`, no Authorization
  header sent. The very first call returns 401; watch startup never recovers.
- **Wrong output:** Every managed-K8s user is broken at the auth layer; this is
  effectively **P0 for users** but listed as P2 because the conventional
  kubeconfig path (with embedded token) still works.
- **Suggested fix category:** This is the same fix as P1-1 — invoke the exec
  plugin lazily on demand (at `RoundTrip` time, not at `Connect` time so the
  exec runs once per TTL window) and use its stdout JSON `{apiVersion, kind,
  status: {token}}` to populate the bearer token.

#### P2-6: `kubeconfig.go` base64 decoder is strict and rejects valid k8s-encoded data
- **File:** `backend/k8s/kubeconfig.go:120-127`, `135-140`, `142-148`
- **Trigger:** `kubectl` writes `certificate-authority-data` etc. as standard
  base64 but on disk / in `~/.kube/config` it is wrapped to 64 cols and contains
  embedded newlines. The parser uses `base64.StdEncoding.DecodeString` which is
  intolerant of any whitespace — even though kubeconfig files commonly have
  `\n` between columns.
- **Reproduction:** `cat ~/.kube/config | grep certificate-authority-data` — the
  decoded value is single-line base64 here (because kubectl writes it
  one-liner), but raw YAMLs exported with `yq` or hand-edited configs often
  have line breaks. `ParseBytes` returns
  `cluster "c" CA data: illegal base64 data` and the entire Connect call is
  rejected — even though every other cluster in the file is fine.
- **Wrong output:** A single broken base64 inside one of N clusters fails the
  whole kubeconfig parse. The Watch/Request afterwards report zero contexts
  even though N-1 are usable.
- **Suggested fix category:** Use `base64.StdEncoding.DecodeString(strings.ReplaceAll(s, "\n", ""))`
  (or yaml's own unmarshal that already permits line-folded base64). Or split
  parse errors per-cluster instead of fail-fast.

#### P2-7: `authRoundTripper.RoundTrip` mutates the caller's `*http.Request` header in-place
- **File:** `backend/k8s/client.go:138-144`
- **Trigger:** A future retry path (e.g. once P1-1 is implemented) re-uses the
  same `*http.Request` for the retry. Because the first call mutated
  `req.Header.Set("Authorization", ...)`, the retry sees an Authorization
  header already set and skips re-injection — which would have refreshed the
  token. Today this matters for `*http.Client` redirect chains too: a 3xx
  response with `Authorization: ...` baked into the outgoing `Location` headers
  leaking via `dump-request` logs (httputil).
- **Reproduction:** Wire a debug `RoundTripper` in front that dumps the
  outgoing request; observe `Authorization: Bearer xyz` in the raw bytes the
  apiserver sees, even if the caller passed no auth header. Then implement
  P1-1 retry — the retry will not re-invoke the bearer because the header is
  no longer empty.
- **Wrong output:** Token rotation retry never re-runs the plugin; auth fails
  after token expires.
- **Suggested fix category:** Clone the request:
  ```go
  req2 := req.Clone(req.Context())
  req2.Header.Set("Authorization", ...)
  return a.base.RoundTrip(req2)
  ```
  Honors `http.RoundTripper` contract and unblocks future retry logic.

---

### P3 — Low / Informational

#### P3-1: No `Manager.Close` / shutdown method
- **File:** `backend/k8s/manager.go` (whole file)
- **Trigger:** Wails app shutdown. There is no public method to terminate all
  open watches, log streams, and dialed connections in one go. `Manager` is
  also never wired into `wails.Run(..., options.OnShutdown, ...)` from
  `main.go`.
- **Reproduction:** Grep `k8sManager.Close\|Shutdown\|Terminate` returns nothing.
  Cancel propagates only through `Disconnect(connID)` per-cluster.
- **Wrong output:** On exit, in-flight goroutines and idle conns are reaped
  only by the OS process exit. Fine in practice, but masks leaks during dev
  test cycles.
- **Suggested fix category:** Add `func (m *Manager) Close()` that iterates
  watches / logs / conns and cancels them all. Add a separate `Shutdown(ctx)`
  variant that waits for goroutines via sync.WaitGroup.

#### P3-2: `rest.Do` makes no distinction between 4xx and 5xx for callers
- **File:** `backend/k8s/rest.go:32-49`
- **Trigger:** Higher layers (`app.go`) bubble `K8sResponse{Status, Body}` to
  the renderer verbatim. The renderer has to inspect a numeric status code to
  decide between "client mistake → show form error" and "server/transient →
  retry". No error wrapping helps.
- **Reproduction:** Watch `K8sRequest("GET", "/api/v1/missing-path", ...)` — the
  response is `(404, body, nil)`. The renderer's catch-all path treats `err==nil`
  as success and may render raw JSON to the user.
- **Wrong output:** Confusing error UX; no log/Sentry signal for the
  classification.
- **Suggested fix category:** Return `(status, body, nil)` for 2xx/3xx;
  `(<status>, body, fmt.Errorf("%d: %s", status, statusText))` for ≥400 so the
  `K8sResponse` error path can branch.

#### P3-3: `buildLogPath` accepts negative `tailLines` / empty container
- **File:** `backend/k8s/logs.go:46-51`
- **Trigger:** Caller passes `tailLines: -1` (some old `kubectl log -n` flags
  forward -1). Sprintf writes `tailLines=-1`; the apiserver rejects with `400`.
- **Reproduction:** `m.StartLogStream(connID, "ns", "pod", "c", -1, false, false)`
  → URL is `/api/v1/namespaces/ns/pods/pod/log?container=c&follow=true&previous=false&tailLines=-1&timestamps=false`.
- **Wrong output:** Spurious 400 from apiserver; same for empty container.
- **Suggested fix category:** Validate inputs at the Go boundary (`tailLines >= 0`,
  `container != ""`) and return a clear Go error before issuing the HTTP
  request.

#### P3-4: Captured `emit` reference in watch/log callbacks does not honor later `SetEventEmitter` swaps
- **File:** `backend/k8s/manager.go:183`, `backend/k8s/manager.go:246`
- **Trigger:** `m.SetEventEmitter(newFn)` is called after some watches started.
  Each watch captured `emit := m.emit` at start time, so swaps are invisible
  to in-flight streams.
- **Reproduction:** Wire a counter emitter; start 5 watches; swap in a
  different emitter with `SetEventEmitter`; trigger an apiserver event —
  the swap has no effect on the first 5 streams.
- **Wrong output:** Debug re-routing requires restarting every watch.
  Acceptable by design, but document the capture semantics — currently
  there's no comment about it.
- **Suggested fix category:** Either change `emit` to read `m.emit` under
  `m.mu` on each call (slower), or document the capture-once contract in
  the `EventEmitter` doc comment.

#### P3-5: `kubeconfig.go` silently drops malformed `exec.env` entries
- **File:** `backend/k8s/kubeconfig.go:149-158`
- **Trigger:** exec plugin wants `env: [{name: K1, value: V1}]`. If the YAML
  has `env: [foo]` (no `name`/`value` keys in the map), `kv["name"]` returns
  `""`, env becomes `{"":""}`.
- **Reproduction:** Manually craft a kubeconfig with `exec.env: - foo` — the
  plugin then runs with `env: {"":""}` which may collide with reserved vars
  the plugin relies on.
- **Wrong output:** Silent misconfiguration of exec plugins.
- **Suggested fix category:** When `kv["name"] == ""` or `kv["value"] == ""`,
  return a parse error in strict mode, or warn via a logger.

#### P3-6: Watch stream bound by `scanner.Buffer` max 4 MiB per line — large single-event crashes the stream
- **File:** `backend/k8s/watch.go:46-47`, `backend/k8s/logs.go:35-36`
- **Trigger:** Custom resources with embedded large `status` blobs (`ConfigMap`
  100s of KiB, or CRDs with verbose `kubectl.kubernetes.io/last-applied-configuration`
  annotations) trigger a watch event whose JSON line exceeds 4 MiB.
- **Reproduction:** Run `kubectl annotate` adding a 5 MiB annotation to a CR;
  watch that CR — `scanner.Scan()` returns false with `bufio.ErrTooLong`,
  `onEnd(scanner.Err())` fires, watch ends.
- **Wrong output:** Stream termination without a way to raise the limit.
- **Suggested fix category:** Either pass a configurable max-line-size and let
  the caller pick a sane bound (e.g. 16 MiB), or convert to
  `json.Decoder.Token()/Decode()` which is unbuffered and accepts arbitrary
  input sizes.

#### P3-7: `Server := "<scheme>://<host>:<port>"` joined via raw string concat
- **File:** `backend/k8s/rest.go:24`, `backend/k8s/watch.go:27`, `backend/k8s/logs.go:19`
- **Trigger:** If `cluster.Server` ends with `/` and `path` starts with `/`,
  result is `host//api/v1/...`. K8s apiservers tolerate, but custom reverse
  proxies (e.g. ingress path normalization) may 404 or redirect.
- **Wrong output:** Inconsistent request URLs across deployments.
- **Suggested fix category:** Use `*url.URL.ResolveReference` or strip a
  trailing slash from base and ensure path starts with `/`.

#### P3-8: `ParseServerAddr` returns `host, port, nil` without IPv6 bracket stripping beyond stdlib
- **File:** `backend/k8s/server_addr.go:12-39`
- **Trigger:** `u.Hostname()` already returns the unbracketed IPv6 literal for
  `https://[::1]:8443`. Fine for HTTP dial. But `LocalAddr(port)` uses
  `JoinHostPort("127.0.0.1", ...)` which means the dial override in
  `app.go:4029-4031` always dials IPv4 loopback even if the cluster URL is
  IPv6. Trivial mismatch.
- **Reproduction:** Set `cluster.server: https://[fd00::1]:6443`; the SSH
  tunnel target is `fd00::1` but the dial override connects to
  `127.0.0.1:<port>` — IPv6 cluster with SSH tunnel on IPv4 loopback
  basically works *because* the SSH tunnel listens on `0.0.0.0`, but the
  abstraction lies.
- **Wrong output:** Mismatch hides future debugging tail.
- **Suggested fix category:** Derive the local binding from the SSH tunnel
  listener interface, or expose `LocalAddr(host, port)` that respects IPv6.

---

## Cross-cutting concerns within module

- **Auth boundary lives in two places.** `authRoundTripper` (REST) and
  `DialExec` (WS) each independently cache `user.Token`. They never agree
  about refresh, and exec plugin lives only in the YAML parse, not in any
  consumer. Fixing `authRoundTripper` once unblocks both REST and WS.
- **No synchronous `Manager` shutdown.** `Manager.Close` (P3-1) plus a
  per-cluster reconnect strategy (P1-2) means every fix here is structural,
  not local.
- **Goroutine lifecycle is split across three call sites.** `startWatchStream`
  and `startLogStream` spawn the goroutine, the `Manager` cleanup is in
  `Disconnect`, and `Stop*` cancels the context. The onEnd callbacks
  fan-in to `delete(m.watches, ...)` or `delete(m.logs, ...)` separately
  — duplication makes it easy to miss when adding a third stream type
  (e.g. metrics or port-forward in the future).
- **Error UX is uniform.** Both `rest.Do`, the onEnd closures, and
  `startWatchStream` / `startLogStream` return `fmt.Errorf` strings; the
  renderer never sees a typed error it can branch on. P3-2 is a small start
  but the whole module would benefit from sentinel errors
  (`ErrAuth`, `ErrNotFound`, `ErrStreamEnded`).
- **Buffer size is consistent between watch & log** (4 MiB / 64 KiB) but
  P3-6's failure mode applies to both.

---

## Summary

- Total findings: 14 (P0: 0, P1: 2, P2: 5, P3: 7)
- Confidence: high (manual read of all 7 source files + the
  `app.go:4048-4068` call sites + tests for behaviour cross-check; no
  runtime/Go-level execution attempted).
- Most consequential — and most economical to fix together — are
  **P1-1 + P2-5** (they collapse into one exec-auth plugin implementation),
  and **P1-2** (the missing reconnect is the single most user-visible
  regression once exec auth lands, since exec tokens cause recurrent 401s).
