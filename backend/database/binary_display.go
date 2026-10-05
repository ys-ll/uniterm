package database

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Drivers hand binary columns back as []byte. Rendering them with string(b)
// turns Guid keys into mojibake in the result grid (Oracle RAW(16) written by
// EF Core / ABP, SQL Server UNIQUEIDENTIFIER). These helpers render such values
// as readable text and convert them back for row edits.

// guidColumns reports, per result column, whether it stores a Guid as 16 bytes
// in .NET Guid.ToByteArray() order: SQL Server UNIQUEIDENTIFIER (go-mssqldb
// returns the wire bytes unswapped by default) and Oracle RAW(16). go-ora does
// not report RAW lengths, so for RAW the value length decides at format time.
func guidColumns(rows *sql.Rows) []bool {
	cts, err := rows.ColumnTypes()
	if err != nil {
		return nil
	}
	out := make([]bool, len(cts))
	for i, ct := range cts {
		switch strings.ToUpper(ct.DatabaseTypeName()) {
		case "UNIQUEIDENTIFIER":
			out[i] = true
		case "RAW":
			n, ok := ct.Length()
			out[i] = !ok || n == 16
		}
	}
	return out
}

func isGuidCol(guids []bool, i int) bool {
	return i < len(guids) && guids[i]
}

// formatBytes renders a driver []byte for display: a Guid string for 16-byte
// Guid columns, the text itself when it is printable UTF-8 (MySQL returns text
// columns as []byte), and 0x-prefixed upper-case hex for anything else.
func formatBytes(b []byte, guid bool) string {
	if guid && len(b) == 16 {
		return FormatDotNetGuid(b)
	}
	if isPrintableText(b) {
		return string(b)
	}
	return "0x" + strings.ToUpper(hex.EncodeToString(b))
}

func isPrintableText(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	for _, c := range b {
		if c < 0x20 && c != '\t' && c != '\n' && c != '\r' {
			return false
		}
	}
	return true
}

// FormatDotNetGuid renders 16 bytes stored in .NET Guid.ToByteArray() order
// (first three groups little-endian) as the canonical lower-case Guid string,
// i.e. the same text Guid.ToString() and the API return.
func FormatDotNetGuid(b []byte) string {
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		b[3], b[2], b[1], b[0], b[5], b[4], b[7], b[6],
		b[8], b[9], b[10], b[11], b[12], b[13], b[14], b[15])
}

// ParseDotNetGuid is the inverse of FormatDotNetGuid. It accepts the
// canonical 36-character form, optionally wrapped in braces.
func ParseDotNetGuid(s string) ([]byte, bool) {
	s = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(s), "{"), "}")
	if len(s) != 36 || s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return nil, false
	}
	h, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(h) != 16 {
		return nil, false
	}
	return []byte{h[3], h[2], h[1], h[0], h[5], h[4], h[7], h[6],
		h[8], h[9], h[10], h[11], h[12], h[13], h[14], h[15]}, true
}

// bytesFromDisplay converts a grid value for a binary column back to bytes:
// a Guid string for 16-byte columns, or the 0x hex form formatBytes produces.
// Anything else (printable text, numbers, nil) is returned unchanged.
func bytesFromDisplay(v any, guid bool) any {
	s, ok := v.(string)
	if !ok {
		return v
	}
	if guid {
		if b, ok := ParseDotNetGuid(s); ok {
			return b
		}
	}
	if len(s) > 2 && (s[:2] == "0x" || s[:2] == "0X") {
		if b, err := hex.DecodeString(s[2:]); err == nil {
			return b
		}
	}
	return v
}
