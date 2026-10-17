// Only frame the initial hex header; the desktop zmodemService is the sole
// protocol parser. Retain a possible split prefix so no client prints it.
export function splitZmodemHeader(data: string): { text: string; pending: string; protocol: string } {
  const header = /\*\*\x18B[0-9a-fA-F]{14}\r[\n\x8a\ufffd]/.exec(data)
  if (header) return { text: data.slice(0, header.index), pending: '', protocol: data.slice(header.index) }
  const partial = /\*{1,2}$|\*\*\x18(?:B(?:[0-9a-fA-F]{0,14}\r?)?)?$/.exec(data)
  return partial
    ? { text: data.slice(0, partial.index), pending: partial[0], protocol: '' }
    : { text: data, pending: '', protocol: '' }
}
