import { describe, expect, it } from 'vitest'
import { EditorState } from '@codemirror/state'
import { decodePropertiesUnicode, encodePropertiesUnicode, restorePropertiesUnicode, propertiesFoldRange } from './propertiesPreview'

describe('Properties Unicode preview', () => {
  it('decodes Chinese comments and values, preserving literal backslashes and control escapes', () => {
    const original = String.raw`## AppName = \u5173\u7cfb\u578b\u6570\u636e\u5e93 \u4e1a\u52a1\u5e93`
    expect(decodePropertiesUnicode(original)).toBe('## AppName = 关系型数据库 业务库')
    expect(decodePropertiesUnicode(String.raw`path=\\u5173\n\u000a\u005c`)).toBe(String.raw`path=\\u5173\n\u000a\u005c`)
    expect(decodePropertiesUnicode(String.raw`emoji=\uD83D\uDE00`)).toBe('emoji=😀')
  })

  it('round-trips unmodified lines exactly, including CRLF and uppercase escapes', () => {
    const original = 'app=\\u5173\\u7CFB\r\n# 原始中文\r\n'
    const decoded = decodePropertiesUnicode(original)
    expect(restorePropertiesUnicode(original, decoded.replace(/\r\n/g, '\n'))).toBe(original)
    expect(restorePropertiesUnicode(original, 'app=关系库\n# 原始中文\n'))
      .toBe('app=\\u5173\\u7cfb\\u5e93\r\n# 原始中文\r\n')
  })

  it('preserves UTF-8 files and encodes edited lines when the original used escapes', () => {
    expect(restorePropertiesUnicode('name=中文\n', 'name=数据库\n')).toBe('name=数据库\n')
    expect(restorePropertiesUnicode('name=\\u4e2d\\u6587\n', 'name=数据库\n')).toBe('name=\\u6570\\u636e\\u5e93\n')
    expect(encodePropertiesUnicode('😀')).toBe('\\ud83d\\ude00')
  })
})

describe('Properties folding', () => {
  it('folds consecutive comment lines and continuation lines, not independent properties', () => {
    const doc = EditorState.create({ doc: '# title\n# comment\nkey=one\\\n  two\\\n  three\nplain=1\nother=2' }).doc
    expect(propertiesFoldRange(doc, doc.line(1).from)).toEqual({ from: doc.line(1).to, to: doc.line(2).to })
    expect(propertiesFoldRange(doc, doc.line(2).from)).toBeNull()
    expect(propertiesFoldRange(doc, doc.line(3).from)).toEqual({ from: doc.line(3).to, to: doc.line(5).to })
    expect(propertiesFoldRange(doc, doc.line(4).from)).toBeNull()
    expect(propertiesFoldRange(doc, doc.line(6).from)).toBeNull()
  })

  it('does not treat an even number of backslashes as a continuation', () => {
    const doc = EditorState.create({ doc: 'path=x\\\\\nnext=y' }).doc
    expect(propertiesFoldRange(doc, 0)).toBeNull()
  })
})
