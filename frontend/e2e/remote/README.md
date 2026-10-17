# Real remote desktop protocol check

Requires a real VNC server on `127.0.0.1:5909`, SPICE TCP on `127.0.0.1:33925`, and QEMU QMP on `127.0.0.1:33926`. The test guest must echo keyboard input to VGA text memory. Run from a disposable fixture, not a user's VM.

1. Build the frontend (`cd frontend && npm run build`) so Go's embedded assets exist.
2. Start Vite from `frontend`: `npx vite --config e2e/remote/vite.config.mjs`.
3. From the repository root, create `.scratch/remote-e2e`, then start:
   `UNITERM_REMOTE_E2E_VITE=http://127.0.0.1:34971 UNITERM_REMOTE_E2E_URL_FILE="$PWD/.scratch/remote-e2e/url" go test -race . -run '^TestWebRemoteDesktopProtocolE2E$' -count=1 -v -timeout=15m`
4. Run `PLAYWRIGHT_MODULE=/absolute/path/to/playwright-core/index.mjs node frontend/e2e/remote/run.mjs`. `CHROME_BINARY`, `QMP_PORT`, `REMOTE_E2E_URL_FILE`, and `REMOTE_E2E_ARTIFACTS` are optional overrides.

The harness mounts the actual Vue components, noVNC/spice-html5, and Go App routes. The surrounding panel store, Wails events, and close RPC facade are test-only adapters. The listener uses a test cookie wrapper; production must register routes with the shared Web authentication listener. This is a protocol/component E2E, not the integrated Wails desktop-shell acceptance test.

Checks: unauthenticated creation denied; rendered first frames; tab-cache restore; keyboard markers read back from real VGA memory; SPICE main/display/inputs/cursor channels; same-origin sockets; manual browser clipboard fallback; reconnect/refresh/close reclaim Go sessions and QEMU sockets. A separate native-origin VNC proxy completes a real RFB handshake before these checks and still receives framebuffer updates afterwards. Screenshots, QMP responses and logs go to `.scratch/remote-e2e`. A minimal BIOS guest has no guest clipboard agent or audio producer; focused Vitest tests cover those adapters without claiming full guest/media protocol validation.
