import type { Text } from '@codemirror/state'

// Decode printable non-ASCII escapes only. Escaped backslashes, ASCII
// delimiters, controls and line breaks retain their original spelling.
export function decodePropertiesUnicode(text: string): string {
  return text.replace(/(\\+)u([0-9a-fA-F]{4})/g, (match, slashes: string, hex: string) => {
    const code = parseInt(hex, 16)
    if (slashes.length % 2 === 0 || code < 0xa0) return match
    return slashes.slice(0, -1) + String.fromCharCode(code)
  })
}

export function encodePropertiesUnicode(text: string): string {
  return text.replace(/(\\*)([^\x00-\x7f])/g, (_, slashes: string, char: string) =>
    slashes.slice(0, slashes.length - slashes.length % 2) + '\\u' + char.charCodeAt(0).toString(16).padStart(4, '0'))
}

// Preserve every unchanged line's original spelling, including escape case,
// comments, literal Unicode and escaped backslashes. Encode new/changed lines
// only when the original document used escaped Unicode.
export function restorePropertiesUnicode(original: string, edited: string): string {
  const normalize = (s: string) => s.replace(/\r\n|\r/g, '\n')
  const source = normalize(original)
  const decoded = decodePropertiesUnicode(source)
  if (decoded === normalize(edited)) return original
  const oldLines = source.split('\n')
  const decodedLines = decoded.split('\n')
  const newLines = normalize(edited).split('\n')
  const originals = new Map<string, Set<string>>()
  decodedLines.forEach((line, i) => {
    const values = originals.get(line) || new Set<string>()
    values.add(oldLines[i]!)
    originals.set(line, values)
  })
  const escaped = source !== decoded
  const lines = newLines.map((line, i) => {
    if (line === decodedLines[i]) return oldLines[i]!
    const values = originals.get(line)
    if (values?.size === 1) return [...values][0]!
    return escaped ? encodePropertiesUnicode(line) : line
  })
  return lines.join(original.includes('\r\n') ? '\r\n' : '\n')
}

const isComment = (line: string) => /^\s*[#!]/.test(line)
const continues = (line: string) => ((line.match(/\\+$/)?.[0].length || 0) % 2) === 1

// Properties is flat rather than hierarchical: fold consecutive comments
// and physical lines of a backslash-continued logical property.
export function propertiesFoldRange(doc: Text, lineStart: number): { from: number; to: number } | null {
  const first = doc.lineAt(lineStart)
  let last = first
  if (isComment(first.text)) {
    if (first.number > 1 && isComment(doc.line(first.number - 1).text)) return null
    while (last.number < doc.lines && isComment(doc.line(last.number + 1).text)) last = doc.line(last.number + 1)
  } else {
    if (first.number > 1) {
      const prev = doc.line(first.number - 1).text
      if (!isComment(prev) && continues(prev)) return null
    }
    while (last.number < doc.lines && continues(last.text)) last = doc.line(last.number + 1)
  }
  return last.number > first.number ? { from: first.to, to: last.to } : null
}
