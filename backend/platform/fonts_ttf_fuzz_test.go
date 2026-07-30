package platform

import "testing"

// QA-023: fuzz test for parseNameTable.
//
// The TTF name-table parser is heavily defensive (bounds checks,
// count caps); fuzzing validates that no input can pan or hang.

func FuzzParseNameTable(f *testing.F) {
	// Seed corpus: empty, single-byte, and a structurally-valid-but-no-name-id-1.
	f.Add([]byte{})
	f.Add([]byte{0x00, 0x01})
	f.Add([]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}) // name header only
	// A minimal valid name-table layout: 6-byte header + 0 records + storageOffset=0.
	f.Add([]byte{
		0x00, 0x00, // platformID
		0x00, 0x00, // encodingID
		0x00, 0x00, // languageID
		0x00, 0x01, // nameID = 1 (family)
		0x00, 0x00, // length
		0x00, 0x00, // offset
	})

	f.Fuzz(func(t *testing.T, data []byte) {
		// parseNameTable takes an offset; pick 0 for the fuzz input.
		_ = parseNameTable(data, 0)
	})
}
