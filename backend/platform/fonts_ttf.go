//go:build windows || darwin

package platform

import "fmt"

// parseFont reads a TTF/OTF/TTC font file and returns (familyName, isMonospace, error).
func parseFont(path string) (string, bool, error) {
	data, err := readFontFile(path)
	if err != nil {
		return "", false, err
	}

	// For .ttc files, use the first sub-font's table directory.
	tableDirOffset := 0
	tag := tag4(data, 0)
	if tag == "ttcf" {
		if len(data) < 16 {
			return "", false, fmt.Errorf("ttc too small")
		}
		offset := u32(data, 12)
		if int(offset)+12 > len(data) {
			return "", false, fmt.Errorf("ttc offset out of range")
		}
		tableDirOffset = int(offset)
		tag = tag4(data, tableDirOffset)
	}

	// Validate font file
	if tag != "true" && tag != "\x00\x01\x00\x00" && tag != "OTTO" {
		return "", false, nil // not a supported font, skip silently
	}

	numTables := int(u16(data, tableDirOffset+4))
	if numTables <= 0 || numTables > 200 {
		return "", false, fmt.Errorf("invalid numTables")
	}

	var nameOffset uint32
	var postOffset, os2Offset, cmapOffset uint32

	for i := 0; i < numTables; i++ {
		off := tableDirOffset + 12 + i*16
		if off+16 > len(data) {
			break
		}
		tableTag := tag4(data, off)
		tableOffset := u32(data, off+8)

		switch tableTag {
		case "name":
			nameOffset = tableOffset
		case "post":
			postOffset = tableOffset
		case "OS/2":
			os2Offset = tableOffset
		case "cmap":
			cmapOffset = tableOffset
		}
	}

	if nameOffset == 0 {
		return "", false, fmt.Errorf("name table not found")
	}

	// Exclude symbol/icon fonts (Wingdings, Font Awesome, etc.).
	if os2Offset != 0 && int(os2Offset)+32 <= len(data) {
		familyClass := u16(data, int(os2Offset)+30)
		if familyClass>>8 == 12 {
			return "", false, nil
		}
	}
	if cmapOffset != 0 && hasWindowsSymbolCmap(data, int(cmapOffset)) {
		return "", false, nil
	}

	// Use the post table's isFixedPitch flag (CJK fonts don't set it).
	if postOffset == 0 || int(postOffset)+16 > len(data) || u32(data, int(postOffset)+12) == 0 {
		return "", false, nil
	}

	// Read font family name from name table (Name ID 1)
	family := parseNameTable(data, int(nameOffset))
	return family, true, nil
}

// hasWindowsSymbolCmap checks whether the cmap table's (platform 3, encoding 0)
// subtable is present — the Windows convention for identifying symbol fonts.
func hasWindowsSymbolCmap(data []byte, cmapOffset int) bool {
	if cmapOffset+4 > len(data) {
		return false
	}
	numTables := int(u16(data, cmapOffset+2))
	if numTables <= 0 || numTables > 100 {
		return false
	}
	for i := 0; i < numTables; i++ {
		recOff := cmapOffset + 4 + i*8
		if recOff+8 > len(data) {
			break
		}
		platformID := u16(data, recOff)
		encodingID := u16(data, recOff+2)
		if platformID == 3 && encodingID == 0 {
			return true
		}
	}
	return false
}

// parseNameTable returns the font family name (Name ID 1) from the name table.
func parseNameTable(data []byte, offset int) string {
	if offset+6 > len(data) {
		return ""
	}

	count := int(u16(data, offset+2))
	storageOffset := offset + int(u16(data, offset+4))
	if count > 500 || storageOffset > len(data) {
		return ""
	}

	var fallback string
	for i := 0; i < count; i++ {
		recOff := offset + 6 + i*12
		if recOff+12 > len(data) {
			break
		}
		platformID := u16(data, recOff)
		encodingID := u16(data, recOff+2)
		nameID := u16(data, recOff+6)
		recLen := int(u16(data, recOff+8))
		recOffset := int(u16(data, recOff+10))

		if nameID != 1 {
			continue
		}

		strOff := storageOffset + recOffset
		if strOff+recLen > len(data) {
			continue
		}
		raw := data[strOff : strOff+recLen]

		if platformID == 3 && encodingID == 1 {
			name := decodeUTF16BE(raw)
			if name == "" {
				continue
			}
			// Prefer localized names (non-ASCII) over English ones.
			if hasNonASCII(name) {
				return name
			}
			if fallback == "" {
				fallback = name
			}
		}
	}

	return fallback
}

func hasNonASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return true
		}
	}
	return false
}

func decodeUTF16BE(data []byte) string {
	if len(data)%2 != 0 {
		return ""
	}
	runes := make([]rune, len(data)/2)
	for i := 0; i < len(data); i += 2 {
		runes[i/2] = rune(u16(data, i))
	}
	return string(runes)
}

// Binary parsing helpers
func tag4(data []byte, off int) string {
	if off+4 > len(data) {
		return ""
	}
	return string(data[off : off+4])
}

func u16(data []byte, off int) uint16 {
	return uint16(data[off])<<8 | uint16(data[off+1])
}

func u32(data []byte, off int) uint32 {
	return uint32(data[off])<<24 | uint32(data[off+1])<<16 | uint32(data[off+2])<<8 | uint32(data[off+3])
}
