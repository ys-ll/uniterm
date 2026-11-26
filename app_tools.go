package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// FormatJSON validates and pretty-prints (or minifies) a JSON document.
// indent is one of "2" / "4" (spaces per level), "tab", or "min" (minify).
func (a *App) FormatJSON(input string, indent string) (string, error) {
	indentStr := "  "
	switch indent {
	case "min":
		indentStr = ""
	case "tab":
		indentStr = "\t"
	case "4":
		indentStr = "    "
	}
	if !json.Valid([]byte(input)) {
		var v any
		err := json.Unmarshal([]byte(input), &v)
		if syntax, ok := err.(*json.SyntaxError); ok {
			// Offset/LastIndex operate on byte offsets, which is fine for
			// locating the error; the column is byte-approximate for UTF-8.
			line := 1 + strings.Count(input[:syntax.Offset], "\n")
			col := int(syntax.Offset) - strings.LastIndexByte(input[:syntax.Offset], '\n')
			return "", fmt.Errorf("JSON syntax error at line %d, column %d: %s", line, col, syntax.Error())
		}
		if err != nil {
			return "", fmt.Errorf("invalid JSON: %w", err)
		}
		return "", fmt.Errorf("invalid JSON")
	}
	if indentStr == "" {
		var v any
		if err := json.Unmarshal([]byte(input), &v); err != nil {
			return "", err
		}
		out, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(out), nil
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(input), "", indentStr); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// HashString computes the common digest algorithms of the input string
// (UTF-8 bytes) and returns their lowercase hex representations.
func (a *App) HashString(input string) (map[string]string, error) {
	data := []byte(input)
	md5Sum := md5.Sum(data)
	sha1Sum := sha1.Sum(data)
	sha256Sum := sha256.Sum256(data)
	sha512Sum := sha512.Sum512(data)
	crc32Sum := crc32.ChecksumIEEE(data)
	return map[string]string{
		"md5":    hex.EncodeToString(md5Sum[:]),
		"sha1":   hex.EncodeToString(sha1Sum[:]),
		"sha256": hex.EncodeToString(sha256Sum[:]),
		"sha512": hex.EncodeToString(sha512Sum[:]),
		"crc32":  fmt.Sprintf("%08x", crc32Sum),
	}, nil
}

// EncodeString encodes or decodes the input using the given encoding
// ("base64" or "hex"). decode=false encodes, decode=true decodes.
// Decoding is tolerant: all whitespace inside the input is stripped, and
// base64 tries the standard / raw / URL-safe variants in turn.
func (a *App) EncodeString(input string, encoding string, decode bool) (string, error) {
	if decode {
		cleaned := strings.Map(func(r rune) rune {
			switch r {
			case ' ', '\n', '\r', '\t':
				return -1
			}
			return r
		}, input)
		switch encoding {
		case "base64":
			var lastErr error
			for _, enc := range []*base64.Encoding{
				base64.StdEncoding, base64.RawStdEncoding,
				base64.URLEncoding, base64.RawURLEncoding,
			} {
				out, err := enc.DecodeString(cleaned)
				if err == nil {
					return string(out), nil
				}
				lastErr = err
			}
			return "", fmt.Errorf("base64 decode failed: %w", lastErr)
		case "hex":
			out, err := hex.DecodeString(cleaned)
			if err != nil {
				return "", fmt.Errorf("hex decode failed: %w", err)
			}
			return string(out), nil
		default:
			return "", fmt.Errorf("unsupported encoding: %q", encoding)
		}
	}
	switch encoding {
	case "base64":
		return base64.StdEncoding.EncodeToString([]byte(input)), nil
	case "hex":
		return hex.EncodeToString([]byte(input)), nil
	default:
		return "", fmt.Errorf("unsupported encoding: %q", encoding)
	}
}

// CronNextRuns parses a standard 5-field (or @-descriptor) cron expression
// and returns the next `count` fire times in local time (RFC3339 strings).
func (a *App) CronNextRuns(expr string, count int) ([]string, error) {
	if count <= 0 || count > 100 {
		count = 5
	}
	sched, err := cron.ParseStandard(strings.TrimSpace(expr))
	if err != nil {
		return nil, fmt.Errorf("invalid cron expression: %w", err)
	}
	out := make([]string, 0, count)
	next := time.Now()
	for i := 0; i < count; i++ {
		next = sched.Next(next)
		out = append(out, next.Format("2006-01-02 15:04:05 MST"))
	}
	return out, nil
}

// YamlToJson converts a YAML document to pretty-printed JSON (2 spaces).
func (a *App) YamlToJson(input string) (string, error) {
	var v any
	if err := yaml.Unmarshal([]byte(input), &v); err != nil {
		return "", fmt.Errorf("invalid YAML: %w", err)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// JsonToYaml converts a JSON document to YAML.
func (a *App) JsonToYaml(input string) (string, error) {
	var v any
	dec := json.NewDecoder(strings.NewReader(input))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return "", fmt.Errorf("invalid JSON: %w", err)
	}
	out, err := yaml.Marshal(convertJSONNumbers(v))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// convertJSONNumbers turns json.Number values into int64/float64 so
// yaml.Marshal renders them as numbers instead of quoted strings.
func convertJSONNumbers(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			t[k] = convertJSONNumbers(val)
		}
		return t
	case []any:
		for i, val := range t {
			t[i] = convertJSONNumbers(val)
		}
		return t
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return i
		}
		if f, err := t.Float64(); err == nil {
			return f
		}
		return t.String()
	default:
		return v
	}
}
