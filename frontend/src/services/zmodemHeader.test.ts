import { expect, it } from 'vitest'
import { splitZmodemHeader } from './zmodemHeader'

it('withholds every fragmented initial header and preserves common surrounding text', () => {
  const header = '**\x18B00000000000000\r\n\x11'
  for (let split = 1; split < header.length; split++) {
    let pending = '', text = '', protocol = ''
    for (const part of ['before\r\n' + header.slice(0, split), header.slice(split)]) {
      if (protocol) { protocol += part; continue }
      const result = splitZmodemHeader(pending + part)
      pending = result.pending
      text += result.text
      protocol += result.protocol
    }
    expect(text).toBe('before\r\n')
    expect(protocol).toBe(header)
  }
  const normal = splitZmodemHeader('plain **B00000000000000\r\n')
  expect(normal.text).toBe('plain **B00000000000000\r\n')
  expect(splitZmodemHeader('**\x18Bang!').text).toBe('**\x18Bang!')
})
