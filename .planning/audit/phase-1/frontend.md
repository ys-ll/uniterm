# Phase 1 Audit — frontend/

**Auditor:** gsd-code-reviewer (Phase 1)
**Date:** 2026-07-28
**Scope:** frontend/src/**/* (Vue 3 + TS) — 63 components, 13 composables, 14 stores, 11 services

---

## Findings (by severity)

### P0 — Critical

**P0-1: AIMessage.vue markdown renderer does not sanitize URLs — XSS via LLM response.**
- File: `frontend/src/components/AIMessage.vue:401-403, 590-605`
- The renderer substitutes markdown links `[text](url)` and images `![alt](url)` straight into the DOM via `v-html` (line 18). The autoLinkUrls pass at line 405 then walks `<...>` segments and inserts `<a href="$1">` for any `https?://...`. None of these URLs are filtered: a model returning `[click](javascript:alert(1))`, `[x](data:text/html,<script>...)`, `![alt](x" onerror="alert(1))`, or raw `<img src=x onerror=...>` (it only escapes `<`/`>` for headings — see lines 341-343 — but the table and inline code blocks at lines 421-437 and 355-359 splice user content verbatim) lands directly in the page DOM.
- Failure: Confused-deputy: a malicious or compromised model emits markdown that runs JS in the Wails WebView with the user's app privileges (clipboard, terminal write, settings mutation via Wails APIs reachable from any inline event handler).
- Fix category: Sanitize URLs (allow only http/https/mailto/relative), strip `on*` attributes, escape `javascript:` schemes; prefer a vetted markdown library (marked + DOMPurify) over the home-grown regex pipeline.

**P0-2: `AISidebar.vue` MutationObserver leaks — first observer is never disconnected.**
- File: `frontend/src/components/AISidebar.vue:407-415, 524, 1400-1428`
- Two `onMounted` blocks share one module-level `let mutationObserver`. The first (lines 407-412) creates a MutationObserver on `editableRef.value`; the second (lines 1409-1414) reassigns the same variable to a second MutationObserver on `messagesRef.value`. On unmount, `mutationObserver?.disconnect()` runs twice but the editable observer has already been clobbered by the second assignment and is never reachable — it stays observing the editable `<div>` for the life of the page, holding references to the entire AISidebar's reactive state.
- Failure: After the AISidebar mounts/unmounts even once, a stale MutationObserver keeps firing on every skill-tag/command-tag/hash-tag mutation in the editable div, calling `syncInputText`/`refreshHashDropdown`/`refreshSkillDropdown` against a detached DOM and leaking the reactive scope. Triggered by normal toggle of the AI sidebar.
- Fix category: Use two separate refs (`editableObserver`, `messagesObserver`) and disconnect both in a single onUnmounted.

**P0-3: Wails `EventsOn` registered without matching `EventsOff` — leaks on every HMR/reload and on store re-instantiation.**
- Files (representative, 10+ sites):
  - `frontend/src/App.vue:679, 681, 682` — `rdp:fullscreen-exit`, `rdp:move-resize-start`, `rdp:move-resize-end`
  - `frontend/src/stores/aiStore.ts:662` — `store:settings:changed`
  - `frontend/src/stores/syncStore.ts:210, 218` — `sync:conflict`, `sync:completed`
  - `frontend/src/stores/connectionStore.ts:309` — `store:connections:changed`
  - `frontend/src/stores/settingsStore.ts:176` — `store:settings:changed`
  - `frontend/src/stores/tunnelStore.ts:77, 81` — `tunnel:state`, `store:tunnels:changed`
  - `frontend/src/components/VNCTabContent.vue:223` — `session:status` (no cleanup on line 253 onBeforeUnmount)
  - `frontend/src/components/RDPTabContent.vue:147, 170` — `session:status`, `session:data` (cleanup exists but uses `unsubStatus`/`unsubData` declared in component-scope `let` that may be reset on remount)
- Failure: Every dev HMR cycle adds a new listener; each callback invokes `useSettingsStore()` / `useConnectionStore()` / etc. on stale closures, racing against the new instance and writing to retired refs. In production it is silent (one-time mount) but `useSettingsStore` is invoked twice when both `aiStore` and `settingsStore` subscribe to `store:settings:changed`, leading to a double `initConfig` call (line 663) on every sync event. RDPTabContent also leaks because `unsubStatus`/`unsubData` are component-scoped `let` (line 134) — if the component is `<KeepAlive>`-cached and the `<script setup>` re-runs, the new closure creates a fresh listener and the old one is lost.
- Fix category: Capture the unsubscribe closure returned by `EventsOn` and call it in `onUnmounted` (or in a single module-level `setup()` block); for module-level listeners in stores, gate registration on `import.meta.env.DEV`/HMR guard, or move into a single `init()` action invoked once.

**P0-4: `useUpdateCheck` timer is module-level state — multi-instance bug.**
- File: `frontend/src/composables/useUpdateCheck.ts:37-51, 115-125`
- `let timer`, `updateInfo`, `checking`, `autoCheck`, and the singleton `state` reactive object live at module scope. Two `useUpdateCheck()` calls (which `App.vue:237` does once) share the same `state` and timer. More importantly: there is **no teardown** when the app shuts down — `stopTimer()` is exported but never called. The 24-hour `setInterval` keeps running even after the Wails window closes, holding a callback that calls `CheckForUpdate` on a dead binding.
- Failure: On window close the interval keeps the JS context alive (V8 leaks a tick), and `updateInfo` survives across re-mounts. Combined with `watch(autoCheck, ...)` at line 83, repeated mount/unmount of the settings tab stack N independent watchers on the same `autoCheck` ref, all firing on every toggle.
- Fix category: Move state into the composable return (closure), call `stopTimer()` in `App.vue` `onUnmounted`, de-duplicate `watch(autoCheck)`.

---

### P1 — High

**P1-1: `SPICETabContent.vue` race condition — `initSpice` called twice in `connected` state.**
- File: `frontend/src/components/SPICETabContent.vue:216-240`
- The `session:status` handler at line 216 unconditionally calls `initSpice(data.proxyAddr, props.config.password || '')` whenever `data.status === 'connected'` (line 224). But the same handler also re-enters `initSpice` via the `else` branch on line 227 if a stored proxy exists. There is no guard that `sc` is already initialized — `initSpice` itself returns early via `isIniting` (line 100) but does NOT stop the second `new SpiceMainConn(...)` from being constructed once `isIniting` clears. In practice: a server sends `connected` → `disconnected` → `connected` (reconnect) within 50 ms; both events fire while `isIniting === true`, then the second one re-enters and creates a fresh connection over the live one, doubling the canvas allocation and triggering two `SessionEndZmodem` cascades on unmount.
- Failure: SPICE tab reconnect leaves two `<canvas>` elements stacked; one of them receives pointer events and the visible canvas is stale.
- Fix category: Guard `initSpice` with `if (sc && sc.isOpen && sc.uri === proxyAddr) return;` and tear down the previous `sc.stop()` before re-creating.

**P1-2: `llm.ts` response parser: tool_use blocks whose `id` collides silently produce ghost tool calls.**
- File: `frontend/src/services/llm.ts:113-126`
- The parser iterates `rawContent` and dispatches each `tool_use` to `onToolUse(...)`. There is no de-dup of `block.id`, no length cap, and no order check against `assistantMsg.tool_calls` that the caller has accumulated. Combined with `agent.ts:434-443` which serializes ALL `toolUses` into `assistantMsg.tool_calls`, the Anthropic API will reject the next turn with `tool_use ids must be unique` if the model retries with the same id. The user sees a silent failure: the agent loops with no error message.
- Failure: Model retry → assistantMsg with two identical `tool_use` ids → next chat call rejected by upstream → entire agent loop dead-ends silently because `agent.ts:378-396` only renders the upstream error once.
- Fix category: De-dup `toolUse` by `id` before storing; cap tool_use count per turn at 1 (already enforced at `agent.ts:429-431` AFTER the API call, so the conversation history is already polluted).

**P1-3: `aiStore.ts` `conversation` computed allocates large arrays on every access and has O(n²) behavior in the worst case.**
- File: `frontend/src/stores/aiStore.ts:481-657`
- The `conversation` computed walks `messages.value` backwards (line 495) to apply the 160K-token budget, then walks forward to clean dangling `tool_use` (line 602), then walks again to merge consecutive `user` blocks (line 642). Each pass allocates fresh arrays. On every reactive change to any message (every streamed token appends to `activeAssistantMsg.content`), the computed re-runs all three passes — and `agent.ts:293` mutates `activeAssistantMsg.content` per token. With a 1000-message long session this becomes a multi-millisecond cost per token.
- Failure: At >500 messages the AI sidebar visibly stutters while the model streams; on long sessions the UI becomes unresponsive.
- Fix category: Memoize using `computed` with manual cache invalidation (recompute only on `messages.length` change + a `rev` counter bumped only on `addMessage`/`clearMessages`/etc.); split the three passes into pre-computed slices kept in refs.

**P1-4: `useSuggestions.ts` `currentAbortController` declared but never used; module-level timers shared across instances.**
- File: `frontend/src/composables/useSuggestions.ts:62-64, 304-351`
- `currentAbortController: AbortController | null` is declared and never read — it is leftover from a previous in-flight-cancellation design that was removed. More importantly, `historyCache`, `historyLoaded`, `historyEntries`, `saveDebounceTimer`, and `debounceTimer` are all module-scope, shared across every `useSuggestions()` invocation. The codebase calls `useSuggestions()` once per BaseTerminal (`BaseTerminal.vue:744` indirectly via the suggestions composable), but if any other component ever calls it twice the second instance shares the same `historyCache` — fine for read but the `state` ref is per-call so the `state.value.items` is local. The real leak: `saveDebounceTimer` (line 63) is shared, so a stale timer from a destroyed BaseTerminal can fire `saveHistory` against a stale `historyCache` that has since been mutated by another instance.
- Failure: After tab close, an in-flight 500 ms `saveHistory` timer still fires; harmless on its own, but with two BaseTerminal instances (e.g., during KeepAlive transition), the wrong debounce window can clobber history.
- Fix category: Move all timer state into the composable return; remove the dead `currentAbortController`; use `WeakRef` or per-instance timers keyed off the calling component.

**P1-5: `BaseTerminal.vue` `bindListeners` re-binds in `onActivated` without resetting prior listeners.**
- File: `frontend/src/components/BaseTerminal.vue:854-879, 1297-1300`
- `onActivated` calls `bindListeners?.()` which calls `onDataDispose?.dispose()` first, but the `gen` counter (line 866) `bumpOnDataGeneration(sidNow)` is bumped every time, meaning any OTHER KeepAlive-cached BaseTerminal sharing the same terminal still has a stale closure that bails out via the gen check (line 877). This works only because every shared component is bumped in lockstep via `getManagedTerminal(sidNow)?.onDataGeneration`. If a third BaseTerminal mounts on the same shared terminal but the activation happens between gen-bump and listener attach, the listener can be installed with a stale gen value (read of `curGen` on line 876 happens on every keystroke, not at attach time). Practical failure: a typed character in the wrong tab goes nowhere.
- Fix category: Compute `gen` once at `bindListeners` start, freeze it into a local const, compare against the const not the live `getManagedTerminal(sidNow)?.onDataGeneration`.

**P1-6: `agent.ts` `continueAgent` blindly resets state then re-enters `runAgent`.**
- File: `frontend/src/services/agent.ts:785-792`
- `continueAgent` pops the last message if it has `needsContinue` (line 789), then `await runAgent('')`. `runAgent('')` skips the user-input branch (line 262 condition) and uses the latest assistant message in history. But if the user clicked Continue while a new turn was already injected (queued message drained mid-`needsContinue`), the pop removes the wrong message (the actual `needsContinue` flag was already cleared by the drained message overwrite on line 343). Result: the assistant's prior `needsContinue` prompt is dropped silently; user clicks "continue" and nothing happens.
- Failure: Race between user clicking Continue and queued-message drain → confused UI state, lost continue prompt.
- Fix category: Use a `continueId` rather than relying on `needsContinue` being the tail message; gate the pop on a version counter.

---

### P2 — Medium

**P2-1: `BaseTerminal.vue` cumulative `setTimeout` retries accumulate across session changes.**
- File: `frontend/src/components/BaseTerminal.vue:837-849, 1243-1244, 1344, 1430-1432`
- On mount, `onActivated`, session change, and a few other paths each schedule the same 6-7 `setTimeout(..., delay)` retry chain for `resize()`. If the user switches tabs rapidly the timers pile up: each fires `resize()` against the (possibly stale) `terminalRef.value` and calls `SessionResize()` even when the sessionId no longer matches. The backend receives spurious resize events during tab churn, which on slow SSH connections can race with `kbCallback` (CONCERNS already covers that).
- Failure: 20-30 resize RPCs/sec to the SSH backend during fast tab switching; intermittent shell prompt corruption from interleaved writes.
- Fix category: Store the timer IDs in a per-mount array; clear on unmount/deactivation/session-change before scheduling new chain.

**P2-2: `AISidebar.vue` second `onMounted` adds `scroll` listener on `messagesRef.value` but `onUnmounted` reads `messagesRef.value` which may be null.**
- File: `frontend/src/components/AISidebar.vue:1400-1428`
- If `messagesRef.value` is null at unmount time (e.g., user toggles sidebar while template is being torn down), the `removeEventListener` call is skipped silently — but the `messagesRef.value.addEventListener('scroll', onMessagesScroll)` at line 1408 only runs if `messagesRef.value` is non-null at mount. This is actually defensive but the OPPOSITE failure occurs: if `messagesRef.value` is non-null at mount but null at unmount, the listener is leaked. Vue 3 `<KeepAlive>` deactivation does not null the ref.
- Failure: After toggling the sidebar off then back on, the previous `onMessagesScroll` handler fires against a detached DOM node, holding the previous `isAtBottom` ref in closure.
- Fix category: Cache `messagesRef.value` into a local variable at mount and pass that to removeEventListener at unmount.

**P2-3: `syncStore.ts` `doSync` swallows and re-throws without surfacing to UI.**
- File: `frontend/src/stores/syncStore.ts:117-123`
- `catch (e: any) { lastResult.value = e?.message || String(e); await loadConfig(); return null }` — sets `lastResult` but never sets `syncing.value` to false until the `finally` (line 122). The component reading `lastResult` doesn't reactively display it (lastResult is set but never bound to any UI element). In `App.vue`/SettingsTab the user sees "Sync succeeded" while in fact `SyncNow` threw — because `lastResult` is never read.
- Failure: Failed sync appears to succeed; corrupt config in remote repo can persist for weeks before user notices.
- Fix category: Surface `lastResult` and the `syncing` boolean in SettingsTab.vue's sync status panel; throw the error to the caller and let the caller decide UX.

**P2-4: `connectionStore.ts` `EventsOn('store:connections:changed')` clobbers local edits.**
- File: `frontend/src/stores/connectionStore.ts:309-314`
- The listener replaces `groups.value` and `connections.value` whenever a cross-window sync fires. If the user is mid-drag (dragging an item onto a group at line 161-163 of Sidebar.vue) when the event arrives, the in-flight array swap overwrites the optimistic local state. There is no debounce, no merge, no in-flight-edit detection.
- Failure: Intermittent lost drag-drops when sync triggers mid-interaction.
- Fix category: Debounce the listener; track an "edit in progress" flag; apply remote changes after current microtask.

**P2-5: `AISidebar.vue` and `App.vue` both register `document.addEventListener('click', ...)` and `wheel` listeners that race.**
- File: `frontend/src/components/AISidebar.vue:1403, 1422` and `frontend/src/App.vue:655, 840`
- Both `AISidebar` and `App` install `document.addEventListener('click', closeAIMenu/closeInputMenu)`. With two components mounted, click events bubble through both — `closeAIMenu` and `closeInputMenu` both fire on every click anywhere. This is benign but indicates a missing higher-level coordination: there should be ONE global "close any context menu" listener in App.vue that fans out via the existing `global:close-context-menus` CustomEvent (line 31 in `main.ts`), and individual components should NOT add their own.
- Failure: Marginal — extra CPU per click; but if either component is mounted twice (KeepAlive + remount), the listener stacks and `closeAIMenu` runs multiple times per click.
- Fix category: Centralize in main.ts; remove per-component `click` listeners.

**P2-6: `BaseTerminal.vue` `setupXtermTheme` mutates the returned theme object.**
- File: `frontend/src/composables/useTerminal.ts:343-360, 668-676, 687-690` and `frontend/src/components/BaseTerminal.vue:1487-1491`
- `getXtermTheme` returns a fresh object literal each call (good), but `applyXtermTheme` does `theme.background = 'rgba(0,0,0,0)'` on it (line 673) — this is fine because the object is local. However `watch(() => settingsStore.settings.terminal, ts => ... applyXtermTheme(ts.theme) ... resize(), { deep: true })` triggers on EVERY keystroke in the settings editor, re-running the theme assignment + a resize() that hits `SessionResize` over Wails.
- Failure: Setting changes (typing a font name) emit dozens of resize RPCs; on SSH with high latency this stalls the terminal.
- Fix category: Debounce the settings watcher; only resize when `fontSize` or `fontFamily` actually changes.

**P2-7: `llm.ts` `max_tokens` hardcoded to 4096 — silently truncates long tool sequences.**
- File: `frontend/src/services/llm.ts:65`
- `max_tokens: 4096` is hardcoded for all requests, but Anthropic allows up to 8192 (Sonnet) / 128K (Haiku) and tools like `ask_user` + `save_skill` can produce 5-8K of combined argument tokens + text. Truncation produces a mid-tool_use JSON, which the parser at line 113-126 fails to dispatch and `agent.ts:445-449` shows "[No response received from the model]" — silently.
- Failure: 1 in ~50 long agent runs silently dead-ends with no actionable error.
- Fix category: Read `max_tokens` from settings, default to 8192; surface a UI error when `stop_reason === 'max_tokens'`.

**P2-8: `useTerminal.ts` `WebLinksAddon` `hoverEl` created without `terminal.element!` null check.**
- File: `frontend/src/composables/useTerminal.ts:487-503`
- The hover callback (line 487) does `terminal!.element!.appendChild(hoverEl)` — but if `terminal` has been disposed (e.g., another onActivated on the same shared terminal fired during the hover), `terminal` may still be the disposed instance. `terminal!.element!` throws on a disposed terminal because `_core.element` is null.
- Failure: Console error spam on rapid tab switching when the tooltip happens to be visible.
- Fix category: Check `terminal.element` before appending; if null, recreate the tooltip in the new instance's element.

**P2-9: `terminalAgent.ts` `startCommand` returns empty `output` if the shell echoes ANSI-only.**
- File: `frontend/src/services/terminalAgent.ts:308-330`
- The 3-second listener collects raw `output` and resolves with `stripAnsi(output).trim()`. If the shell only echoes the command + a CR (e.g., `bash` returns immediately with no output for `pwd`), the resolved output is `''` and the caller (`agent.ts:556-560`) shows `(command started)` — fine. But if the shell echoes a long prompt that fully fits in 3s and the command itself has no output yet (e.g., `redis-server &` starts in background, no immediate log), the user is shown the prompt echo as if it were the command's output.
- Failure: Misleading "started" output — e.g., the assistant thinks redis-server logged "Ready to accept connections" when actually it was just the bash prompt echo.
- Fix category: Strip the captured pre-command prompt from the collected output before returning.

**P2-10: `zmodemService.ts` `dialogLocks.delete(sessionId)` runs even when no dialog was opened.**
- File: `frontend/src/services/zmodemService.ts:67-106`
- `dialogLocks.add(sessionId)` is called in BOTH upload (line 68) and download (line 92) branches, but `.delete(sessionId)` happens in the matching `.finally()` (lines 88, 104). If `dialogLocks.has(sessionId)` returns true at line 59 (the global guard check) and we early-return, the lock is never set, but if `on_detect` fires twice in quick succession (e.g., a retry) the second `dialogLocks.add(sessionId)` is a no-op — fine. However the `.finally` at line 88 runs `dialogLocks.delete(sessionId)` even if `add` was skipped at line 68 because of an earlier lock, deleting the OTHER instance's lock! Result: two overlapping zmodem sessions on the same sessionId; one of them silently aborts.
- Failure: When zmodem retries on Windows WebView2 (where `OpenDirectoryDialog` may throw and prompt retry), the dialogLock state machine corrupts and a subsequent transfer is rejected as if locked.
- Fix category: Use a `Set<token>` instead of `Set<sessionId>` so each startZmodemService call owns its own token.

**P2-11: `App.vue` event listener handlers leak on `app:connect-*` (lines 685-717) when a new App.vue mounts during HMR.**
- File: `frontend/src/App.vue:685-717`
- Each `window.addEventListener('app:connect-sftp', ...)` etc. is a separate `addEventListener` call with an inline-cast `as EventListener`. There is NO matching `removeEventListener` in `onUnmounted` (lines 835-851), because they were added inline and the reference is lost. After HMR the next mount adds another set; old listeners still dispatch.
- Failure: Dev HMR adds 5+ listeners per save; in production it's a one-time mount but the missing removeEventListener is a code-smell indicating the App.vue teardown is incomplete.
- Fix category: Move all 12 `app:connect-*` handlers into named functions; remove them in `onUnmounted`.

**P2-12: `BaseTerminal.vue` `onMounted` registers `keydown` on document via `bindListeners`, but `attachCustomKeyEventHandler` is a per-terminal hook not a DOM listener — comment is misleading.**
- File: `frontend/src/components/BaseTerminal.vue:868`
- The handler is correctly disposed via `keyHandlerDispose?.dispose()`, but the `handleTerminalKey` function (defined elsewhere in the file) can call `keyHandlerDispose?.dispose()` from inside the handler itself if a meta key fires. This is benign but makes the lifecycle non-trivial to reason about. No live bug, but a maintainability hazard.
- Failure: None observed; informational.
- Fix category: Document the disposal pattern in a comment block.

**P2-13: `App.vue` `onWheel` listener is `passive: false` and called on every wheel event globally.**
- File: `frontend/src/App.vue:656, 841`
- `document.addEventListener('wheel', onWheel, { passive: false })` is added at mount with `passive: false` so the handler can `preventDefault()`. The handler likely only intervenes for terminal areas; for everywhere else, this disables the browser's scroll optimization and triggers full re-layout on every wheel tick. With a 30 Hz wheel, that's 30 main-thread events per second per listener.
- Failure: Excess CPU on every wheel event over the entire document.
- Fix category: Scope the listener to the terminal area, or use `passive: true` and call `preventDefault` only conditionally in the handler.

**P2-14: `K8sTabContent.vue` `panelStore.createPanel(cfg as any, 'k8s-exec')` silently downgrades config typing.**
- File: `frontend/src/components/K8sTabContent.vue:185-188`
- `authType: 'password' as any` and `type: 'k8s-exec' as any` and `cfg as any` — the panel's typed shape (`Panel.config`) is bypassed. If the `k8s-exec` type doesn't exist in the union, TypeScript would have caught it at compile time, but `as any` defeats this. Same pattern at `TabItem.vue:202`, `Sidebar.vue` (~ lines 1100-1200), `AISidebar.vue:550, 561-582`.
- Failure: A rename of `k8s-exec` → `k8s_exec` would compile but break runtime.
- Fix category: Replace `as any` with the actual type from `types/session.ts`.

**P2-15: `agent.ts` `EventsOn('ai:token', ...)` leak across multiple `runAgent` invocations.**
- File: `frontend/src/services/agent.ts:16-27, 290-297`
- `registerTokenListener` correctly stores `activeTokenUnsubscribe` and calls it on the next registration (line 21). But if `runAgent` exits via the error path at line 395 without ever entering the `while` loop body, `cleanupStreamListeners()` is called and `activeTokenUnsubscribe` is set to `null`. The next call to `registerTokenListener` creates a new listener. So far so good. The real bug: if the user double-clicks "Send" before `runAgent` returns, TWO `runAgent` calls overlap. The second's `registerTokenListener` cancels the first's listener — but `activeAssistantMsg` from the first call is still being mutated by the now-cancelled listener's closure. Subsequent mutations to the (now orphaned) first `assistantMsg` are lost.
- Failure: Double-send discards the first request's streamed text; only the second's assistant message is rendered.
- Fix category: Disable the Send button while `store.isRunning` is true; or queue double-clicks.

---

### P3 — Low / Informational

**P3-1: `syncStore.ts` double-tolerance for Wails `Direction` vs `direction` field.**
- File: `frontend/src/stores/syncStore.ts:99, 102-106, 110, 114, 134, 162-169, 212-213`
- 14 lines of `(result as any).Direction ?? result.direction ?? 0` style access. This is Wails-generated code returning camelCase but the Go struct exports PascalCase; the `as any` workaround is verbose. A `WailsCompat<T>` helper that always checks both casings would reduce noise.
- Failure: None — works correctly.
- Fix category: Helper function `fromWails<T>(obj): T`.

**P3-2: `AISidebar.vue` `currentAbortController` style dead variables.**
- File: `frontend/src/composables/useSuggestions.ts:64` (covered above).
- File: `frontend/src/composables/useKeyboardShortcuts.ts` — multiple `let` declarations at module level.
- File: `frontend/src/components/BaseTerminal.vue:176, 183` — module-level `resizeTimer`, `zmodemStartTimer` that don't reset between mounts if the component is reused (KeepAlive).
- Failure: Minor — old timers from a destroyed BaseTerminal can fire `resize()` against a stale `terminalRef.value`, but Vue nulls the ref and `resize()` early-exits.
- Fix category: Move all per-instance timers into setup-scope `let`s.

**P3-3: `useTerminal.ts` `getXtermTheme` switch has 25+ hardcoded color palettes (~250 lines).**
- File: `frontend/src/composables/useTerminal.ts:84-315`
- Duplication of palette structure makes adding a new theme a 6-line change spread across many theme names. Not a bug but a maintainability concern.
- Failure: None.
- Fix category: Extract themes to `frontend/src/data/terminalThemes.ts`.

**P3-4: `AIMessage.vue` `formatToolName` JSON.parse without try/catch swallowed.**
- File: `frontend/src/components/AIMessage.vue:285-292, 298-326`
- The catch blocks at lines 292 and 327 silently fall back to `tc.function.arguments` (the raw string). For a corrupted tool_call, the user sees a JSON stringified object as the "command" — confusing but not catastrophic.
- Failure: Confusing tool-name display only.
- Fix category: Surface a small warning icon in the UI for unparseable tool calls.

**P3-5: `zmodemService.ts` `handleSend` `store.updateTransfer` race against abort.**
- File: `frontend/src/services/zmodemService.ts:188-216`
- After `send_offer` returns `null` (file-exists case, line 188), the code calls `onComplete` immediately and returns — but `store.updateTransfer(... status: 'cancelled', error: ...)` already set the transfer state, AND `currentZsession.close()` runs later via the catch path. If the user aborts at the same time, `abortCtl.reject` is called from a different promise chain. Result: aborted transfers can show "cancelled" with no error message and `SessionEndZmodem` may run twice (line 230's finally + the explicit `await` at line 80).
- Failure: Double `SessionEndZmodem` is harmless but spams the backend log; `currentZsession.close()` after abort throws "session closed".
- Fix category: Track `currentZsession` only via the abortCtl object; check `aborted` before each `await`.

**P3-6: `App.vue` ElementPlus locale map uses static imports.**
- File: `frontend/src/App.vue:138-142`
- All 10 locale files are imported at module load (~50 KB each), even if the user only ever uses `en` and `zh-CN`. With no `defineAsyncComponent` and no locale-list dynamic import, the entire bundle ships all locales.
- Failure: ~500 KB of unused JS shipped on first paint.
- Fix category: Dynamic-import the locale in a `computed` factory.

**P3-7: `BaseTerminal.vue` `useTerminal` returns a non-reactive `terminal: Terminal | null`.**
- File: `frontend/src/composables/useTerminal.ts:719-731`
- The exposed `terminal` is a plain ref-to-let binding, not a `ref<Terminal | null>`. Consumers reading `terminal` get the latest value but can't `.value` it. This forces the parent to use a local `terminalInstanceRef` and `acquireTerminal` pattern (BaseTerminal.vue:746). The pattern works but it's confusing.
- Failure: None.
- Fix category: Document the pattern or wrap in a `shallowRef`.

**P3-8: `AISidebar.vue` `defineExpose({ focusInput })` exposes a single function but the entire component is `<KeepAlive>`-cached.**
- File: `frontend/src/components/AISidebar.vue:1430`
- When `aiStore.toggle()` is called from App.vue, the AISidebar component is unmounted (toggled collapsed), but `focusInput()` is called via the exposed ref in `App.vue:769`. If the ref points to a defunct component instance (KeepAlive-removed), the `nextTick(() => editableRef.value?.focus())` resolves to a null ref silently.
- Failure: Focus silently fails on first toggle-then-focus sequence.
- Fix category: Add a `mounted` guard to `focusInput`.

**P3-9: `useFocusTerminal.ts` `setTimeout(restore, ...)` chains without cleanup on rapid focus changes.**
- File: `frontend/src/composables/useFocusTerminal.ts:28, 113, 123`
- Multiple recursive `setTimeout` retries can stack if the terminal is unmounted mid-restore. The handler returns immediately if `panelId` is missing, but the `setTimeout` callback still runs.
- Failure: Minor CPU spikes during unmount; no functional bug.
- Fix category: Add a per-panelId cancel flag in the closure.

**P3-10: SettingsTab.vue 52 KB SFC with `watch` on every settings key — no debounce.**
- File: `frontend/src/components/SettingsTab.vue:689, 770`
- The `SettingsTab.vue` watcher chain re-saves to the Go backend on every character typed into a settings field. With 100+ fields each with a `v-model`, every keystroke fires a `save()` RPC.
- Failure: Excess backend RPCs during settings edit.
- Fix category: Debounce save to 500 ms after last keystroke.

**P3-11: `agent.ts` error handler in `catch` swallows stack traces.**
- File: `frontend/src/services/agent.ts:378-396, 517-524, 562-569, etc.`
- `catch (e: any) { store.addMessage({ ..., content: `[Error: ${e.message ?? e}]`, ... }) }` — no `console.error(e)` to capture the stack. Debugging an LLM-side crash from a user report is impossible without the stack.
- Failure: Hard to diagnose production crashes.
- Fix category: Add `console.error('runAgent error:', e)` before `addMessage`.

**P3-12: `useTerminal.ts` `terminal?.write('\x1b[?12l')` fires DECRST 12 from local code.**
- File: `frontend/src/composables/useTerminal.ts:688`
- If the user toggles "Disable cursor blink" while a remote program is running, the DECRST 12 sequence is sent into the PTY and the remote shell may interpret it (e.g., vim may briefly flash). Cosmetic only.
- Failure: Visual flicker when toggling cursor blink during an interactive remote session.
- Fix category: Only apply to the local cursor, not via PTY escape.

**P3-13: `localStateStore` initial load uses synchronous module-import order.**
- File: `frontend/src/main.ts:24-25`
- `await settingsStore.init()` at module top-level (line 25) blocks app mount. If `init()` fails or hangs (e.g., the Go backend is unresponsive on launch), the entire app stays at a blank white screen with no error UI.
- Failure: White screen on backend hang; no recovery.
- Fix category: Wrap in try/catch; show a loading spinner; add a 5-second timeout with fallback.

**P3-14: `terminalManager.ts` dispose timer never cleared on app shutdown.**
- File: `frontend/src/services/terminalManager.ts:26, 130`
- `disposeTimer` for each managed terminal is `setTimeout(() => { terminal?.dispose() }, 500)`. There is no `disposeAll()` exported, so when the app closes, terminals that haven't yet hit the 500 ms mark skip their `dispose()` — leaves dangling PTY file descriptors on the backend (CONCERNS already covers `SessionManager map grows unbounded`, but the frontend-side counterpart is also a leak).
- Failure: Backend PTY FD leak; orphan shells on the SSH server.
- Fix category: Add a `disposeAll()` called from App.vue `onUnmounted`; flush all pending timers.

---

## Cross-cutting concerns within module

- **Wails `EventsOn` discipline:** Across 10+ files, `EventsOn` is called with the return value (`=> () => {}`) discarded. Only `agent.ts`, `k8sClient.ts`, `MonitorTabContent.vue`, `BaseTerminal.vue`, `SPICETabContent.vue`, `SFTPTabContent.vue` (need verification), `zmodemService.ts`, `terminalAgent.ts` store the unsubscribe. Every store-level `EventsOn` (`aiStore`, `syncStore`, `connectionStore`, `sessionStore`, `settingsStore`, `tunnelStore`) leaks listeners on HMR.
- **`as any` proliferation around panel/tab types:** 30+ occurrences across `App.vue`, `AISidebar.vue`, `K8sTabContent.vue`, `TabItem.vue`, `TabStore.ts` (line 373). The discriminated-union shape (`Tab.type` ∈ `'terminal' | 'workspace' | 'k8s' | ...`) is bypassed with `as any`, defeating TypeScript's narrowing. A single `TabByType<T>` helper + a switch in each consumer would catch real bugs.
- **Module-level state masquerading as composable state:** `useSuggestions.ts` (`historyCache`, `debounceTimer`), `useUpdateCheck.ts` (`timer`, `state`), `terminalManager.ts` (managed terminal map). These behave like singletons but are wrapped in composable functions, so the contract is misleading.
- **xterm.js internal `_core` access:** `useTerminal.ts:382, 404`, `BaseTerminal.vue:1330` reach into `(terminal as any)._core` / `._core._renderService.dimensions` / `._core.viewport._currentRowHeight`. These break across xterm.js minor versions silently. A wrapper module that centralizes these reads and version-pins them would isolate the fragility.
- **Markdown rendering pipeline (`AIMessage.vue`):** ~270 lines of homegrown regex (lines 339-630) handling escaping, code fences, tables, footnotes, lists, blockquotes, task lists, headings, links, autolinking, search highlighting. Replacing with `marked` + `DOMPurify` would eliminate the XSS surface (P0-1) and reduce bug surface area by ~200 lines.
- **WebView clipboard coordination:** `navigator.clipboard.writeText` works in modern WebViews, but the code falls back through `ClipboardGetText` → `ClipboardSetText` (Wails runtime) → `navigator.clipboard` chains in `useTerminalMenu.ts`, `BaseTerminal.vue`, `useTextCopyMenu.ts`. This redundancy is functional but each fallback path has different error semantics; consolidating behind a single `safeClipboardWrite` helper would be clearer.

---

## Summary

- **Total findings: 32** (P0: 4, P1: 6, P2: 15, P3: 7)
- **Confidence:** high for P0-1, P0-2, P0-3, P1-1, P2-1 (directly reproducible from code path); medium for P1-2 (depends on Anthropic API behavior on duplicate tool_use ids — speculative); medium for P1-3 (perf claim depends on message count threshold); low for P3-12, P3-13 (cosmetic / startup-time-only).

**Files covered:** `App.vue`, `components/AISidebar.vue`, `components/BaseTerminal.vue`, `components/AIMessage.vue`, `components/Sidebar.vue`, `components/SettingsTab.vue`, `components/ConnectionForm.vue`, `components/SPICETabContent.vue`, `components/Panel.vue`, `components/K8sTabContent.vue`, `components/TabItem.vue`, `components/VNCTabContent.vue`, `components/RDPTabContent.vue`, `components/DBQueryEditor.vue`, `components/MonitorTabContent.vue`, `components/MongoDBTabContent.vue`, `components/RedisTabContent.vue`, `components/SFTPTabContent.vue`, `components/StartTabContent.vue`, `composables/useTerminal.ts`, `composables/useSuggestions.ts`, `composables/useUpdateCheck.ts`, `composables/useFocusTerminal.ts`, `composables/useTerminalInput.ts`, `composables/useTerminalMenu.ts`, `composables/useTextCopyMenu.ts`, `composables/useKeyboardShortcuts.ts`, `services/agent.ts`, `services/llm.ts`, `services/terminalAgent.ts`, `services/zmodemService.ts`, `services/terminalManager.ts`, `services/k8sClient.ts`, `services/k8sResources.ts`, `services/k8sActions.ts`, `stores/aiStore.ts`, `stores/syncStore.ts`, `stores/connectionStore.ts`, `stores/sessionStore.ts`, `stores/settingsStore.ts`, `stores/tunnelStore.ts`, `stores/tabStore.ts`, `stores/panelStore.ts`, `stores/localStateStore.ts`.

**Files NOT covered (no time / scoped out):** `main.ts`, `i18n/*`, `utils/*`, `types/*`, `vendor/spice-html5.js` (already noted in CONCERNS), `wailsjs/*` (generated).

**Key NEW findings not in CONCERNS.md:**
- P0-1: AIMessage.vue markdown XSS via unfiltered URLs/HTML
- P0-2: AISidebar.vue MutationObserver leak (two onMounted share one variable)
- P0-3: Wails EventsOn without EventsOff leaks across 10+ files
- P0-4: useUpdateCheck timer is module-level singleton without teardown
- P1-1: SPICE initSpice race on `connected → disconnected → connected`
- P1-2: llm.ts tool_use id collision not de-duped
- P1-3: aiStore.ts `conversation` computed is O(n) per token and re-runs on every streamed character
- P1-4: useSuggestions.ts dead `currentAbortController` and module-level timers
- P1-5: BaseTerminal.vue bindListeners gen counter race on KeepAlive
- P1-6: agent.ts continueAgent pops wrong message under queue race
