import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath } from 'node:url'
export default defineConfig({
  plugins: [{ name: 'remote-e2e-shell', enforce: 'pre', resolveId(source, importer) {
    if (importer && (source === '@wailsio/runtime' || /(?:stores\/(panelStore|sessionStore)|bindings\/github.com\/ys-ll\/uniterm\/app)$/.test(source))) {
      return fileURLToPath(new URL('./mocks.ts', import.meta.url))
    }
  } }, vue()],
  server: { host: '127.0.0.1', port: 34971, strictPort: true, hmr: false },
})
