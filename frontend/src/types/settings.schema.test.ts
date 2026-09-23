// Schema-drift guard: settings are persisted through the Go structs in
// backend/store/settings_store.go. A frontend settings key without a
// matching Go json tag is silently dropped by the JSON round-trip, so its
// setting resets on every restart (this bit showTabShortcutHints and
// hostListMenuStyle). These tests fail the moment a new DEFAULT_SETTINGS
// key is not mirrored on the Go side.
import { describe, expect, it } from 'vitest'
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { DEFAULT_SETTINGS } from './settings'

const here = dirname(fileURLToPath(import.meta.url))
const goStructsSrc = readFileSync(resolve(here, '../../../backend/store/settings_store.go'), 'utf-8')
const settingsStoreSrc = readFileSync(resolve(here, '../stores/settingsStore.ts'), 'utf-8')

// Keys persisted as opaque maps on the Go side (map[string]... transparently
// round-trips every child key): only the top-level key needs a Go tag.
const OPAQUE_MAP_KEYS = new Set(['keyboard', 'sidebarTabs', 'bottomBarTabs'])

function collectLeafKeys(obj: object, prefix = '', out = new Set<string>()): Set<string> {
  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (OPAQUE_MAP_KEYS.has(path)) {
      out.add(path)
    } else if (value && typeof value === 'object' && !Array.isArray(value)) {
      collectLeafKeys(value, path, out)
    } else {
      out.add(path)
    }
  }
  return out
}

// Extract the json tag names from a Go struct block (brace-matched).
function goStructTags(src: string, structName: string): Set<string> {
  const marker = `type ${structName} struct {`
  const start = src.indexOf(marker)
  if (start === -1) throw new Error(`struct ${structName} not found in settings_store.go`)
  const open = src.indexOf('{', start)
  let depth = 0
  let end = open
  for (let i = open; i < src.length; i++) {
    if (src[i] === '{') depth++
    else if (src[i] === '}') {
      depth--
      if (depth === 0) {
        end = i
        break
      }
    }
  }
  const tags = new Set<string>()
  for (const m of src.slice(open, end).matchAll(/json:"([^"]+)"/g)) {
    tags.add(m[1].split(',')[0])
  }
  return tags
}

// All structs that mirror a slice of AppSettings, and which top-level key
// each nested struct serves. Everything else lives directly on AppSettings.
// When a nested settings struct is added on the Go side, register it here.
const NESTED_STRUCTS: Record<string, string> = {
  terminal: 'TerminalSettings',
  ai: 'AISettings',
  sftpBookmarks: 'SFTPBookmarks'
}

const tagsByStruct: Record<string, Set<string>> = {}
for (const s of new Set(['AppSettings', ...Object.values(NESTED_STRUCTS)])) {
  tagsByStruct[s] = goStructTags(goStructsSrc, s)
}

// A frontend leaf key passes when its last segment has a json tag in the Go
// struct that stores it (top-level key → AppSettings or the nested struct).
function hasGoTag(path: string): boolean {
  const parts = path.split('.')
  const structTags = tagsByStruct[NESTED_STRUCTS[parts[0]] ?? 'AppSettings']
  return structTags.has(parts[parts.length - 1])
}

describe('settings schema parity (frontend ↔ backend/store/settings_store.go)', () => {
  it('every leaf key in DEFAULT_SETTINGS has a Go json tag', () => {
    const missing = [...collectLeafKeys(DEFAULT_SETTINGS)].filter(k => !hasGoTag(k))
    expect(
      missing,
      `settings keys dropped by the Go JSON round-trip — add them to backend/store/settings_store.go: ${missing.join(', ')}`
    ).toEqual([])
  })

  it('mergeSettings in settingsStore.ts re-maps every top-level key', () => {
    const start = settingsStoreSrc.indexOf('function mergeSettings')
    const body = settingsStoreSrc.slice(start, settingsStoreSrc.indexOf('\n}', start))
    const missing = Object.keys(DEFAULT_SETTINGS).filter(k => !new RegExp(`\\b${k}\\s*:`).test(body))
    expect(
      missing,
      `keys missing from mergeSettings (they reset to defaults on load): ${missing.join(', ')}`
    ).toEqual([])
  })
})
