package session

import "testing"

// QA-023: fuzz tests for the ANSI escape parser.  Both run the
// constants and bounds: a giant int must clamp to maxCSIParam, and
// a malformed CSI sequence must yield 0/false rather than crash.

func FuzzScanEscape(f *testing.F) {
	f.Add([]byte{0x1b, '['})                     // truncated CSI
	f.Add([]byte{0x1b, '[', '1', ';', '2', 'H'}) // CSI 1;2 H
	f.Add([]byte{0x1b, '[', '?'})                // DEC private
	f.Add([]byte{0x1b, 'N', 'x'})                // SS2
	f.Add([]byte{0x1b, ']'})                     // OSC truncated
	f.Add([]byte{0x1b})                          // bare ESC
	f.Add([]byte{0x1b, 0x7F})                    // ESC + high-bit
	f.Add([]byte{0x1b, '[', 0x40})               // final byte at minimum

	f.Fuzz(func(t *testing.T, data []byte) {
		// scanEscape can return (start, false) when input is
		// incomplete; ensure we don't loop or panic.
		pos := 0
		for pos < len(data) {
			next, ok := scanEscape(data, pos)
			if !ok {
				break
			}
			if next <= pos {
				t.Fatalf("scanEscape did not advance: pos=%d → %d", pos, next)
			}
			pos = next
			if pos > len(data) {
				t.Fatalf("scanEscape overrun: pos=%d > len=%d", pos, len(data))
			}
		}
	})
}

func FuzzParseCSIParam(f *testing.F) {
	f.Add([]byte(""))
	f.Add([]byte("1"))
	f.Add([]byte("1;2;3"))
	f.Add([]byte("0;0;0"))
	f.Add([]byte(";"))
	f.Add([]byte(";;"))
	f.Add([]byte("abc"))
	f.Add([]byte("99999999999999999999999999")) // huge number
	f.Add([]byte("-1"))                         // negative
	f.Add([]byte("00001"))
	f.Add([]byte("1;;")) // empty slots

	f.Fuzz(func(t *testing.T, params []byte) {
		// Single-shot call: must not panic, must not exceed maxCSIParam.
		const defaultVal = 1
		for idx := 0; idx < 4; idx++ {
			n := parseCSIParam(params, idx, defaultVal)
			if n < 0 {
				t.Errorf("parseCSIParam negative: params=%q idx=%d → %d", params, idx, n)
			}
			if n > maxCSIParam {
				t.Errorf("parseCSIParam exceeds cap: params=%q idx=%d → %d", params, idx, n)
			}
		}
	})
}
