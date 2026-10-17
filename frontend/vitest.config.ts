import { configDefaults, defineConfig, mergeConfig } from 'vitest/config'
import viteConfig from './vite.config.ts'

export default mergeConfig(viteConfig, defineConfig({
  test: { exclude: [...configDefaults.exclude, '**/*.browser.test.ts'] },
}))
