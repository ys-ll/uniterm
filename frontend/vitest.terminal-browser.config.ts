import { defineConfig } from 'vitest/config'
import { playwright } from '@vitest/browser-playwright'

export default defineConfig({
  optimizeDeps: { include: ['@wailsio/runtime', '@xterm/addon-fit', '@xterm/addon-search', '@xterm/addon-web-links', 'pinia', 'vue'] },
  test: {
    include: ['src/services/terminal*.browser.test.ts'],
    browser: {
      enabled: true,
      headless: true,
      provider: playwright({ launchOptions: { executablePath: process.env.CHROME_PATH } }),
      instances: [{ browser: 'chromium' }],
    },
  },
})
