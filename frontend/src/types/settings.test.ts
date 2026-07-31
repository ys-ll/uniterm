import { describe, it, expect } from 'vitest'
import { readFileSync, readdirSync } from 'node:fs'
import { join } from 'node:path'
import { SUPPORTED_LOCALES } from './settings'

// Regression guard for PR-08 (Win11 + macOS26 UI themes).
// Every theme added to the Theme union must have a matching
// settings.themeXxx key in every supported locale, otherwise
// the picker falls back to the raw i18n key as the label.

const ALL_THEMES = ['dark', 'deep-blue', 'light', 'system', 'win11', 'macos26'] as const

// Map Theme -> i18n key suffix used in locale files.
// Hand-maintained because 'macos26' capitalizes to 'MacOS26' (not 'Macos26'),
// and 'deep-blue' would otherwise lose its hyphen handling.
const THEME_KEYS: Record<(typeof ALL_THEMES)[number], string> = {
  'dark': 'themeDark',
  'deep-blue': 'themeDeepBlue',
  'light': 'themeLight',
  'system': 'themeSystem',
  'win11': 'themeWin11',
  'macos26': 'themeMacOS26',
}

const localesDir = join(__dirname, '..', 'i18n', 'locales')

function loadLocale(name: string): Record<string, unknown> {
  return JSON.parse(readFileSync(join(localesDir, `${name}.json`), 'utf-8'))
}

describe('settings theme picker / locale registration', () => {
  it('every Theme has a settings.theme* key in every supported locale', () => {
    const missing: string[] = []
    for (const locale of SUPPORTED_LOCALES) {
      const data = loadLocale(locale) as Record<string, unknown>
      for (const theme of ALL_THEMES) {
        const key = `settings.${THEME_KEYS[theme]}`
        if (!(key in data)) {
          missing.push(`${locale}: ${key}`)
        }
      }
    }
    expect(missing).toEqual([])
  })

  it('every supported locale has a non-empty label for Win11 and macOS26', () => {
    for (const locale of SUPPORTED_LOCALES) {
      const data = loadLocale(locale) as Record<string, string>
      expect(data['settings.themeWin11']?.length ?? 0).toBeGreaterThan(0)
      expect(data['settings.themeMacOS26']?.length ?? 0).toBeGreaterThan(0)
    }
  })

  it('locale files on disk match SUPPORTED_LOCALES (no missing, no orphans)', () => {
    const files = readdirSync(localesDir)
      .filter(f => f.endsWith('.json'))
      .map(f => f.replace(/\.json$/, ''))
      .sort()
    const supported = [...SUPPORTED_LOCALES].sort()
    expect(files).toEqual(supported)
  })
})
