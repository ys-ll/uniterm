package database

import (
	"strings"
	"testing"
)

// Regression test for AUDIT-database-01..05 — SQL injection in identifier quoting.
// Covers payloads that previously escaped the quoting in MySQL/Postgres/rqlite.
func TestSafeMyIdent(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
		want    string
	}{
		{"plain", "users", false, "`users`"},
		{"backtick injection", "`; DROP TABLE users; --", false, "```; DROP TABLE users; --`"},
		{"empty", "", true, ""},
		{"nul byte", "foo\x00bar", true, ""},
		{"path traversal", "../etc/passwd", true, ""},
		{"too long", strings.Repeat("a", 200), true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SafeMyIdent(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Errorf("SafeMyIdent(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSafePgIdent(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "users", `"users"`},
		{"double-quote injection", `x"; DROP TABLE users; --`, `"x""; DROP TABLE users; --"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SafePgIdent(tc.input)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tc.want {
				t.Errorf("SafePgIdent(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// Regression test for AUDIT-database-04 — `SET DEFAULT %s` injection.
func TestSafeDefaultLiteral(t *testing.T) {
	cases := []struct {
		input   string
		wantErr bool
	}{
		{"NULL", false},
		{"42", false},
		{"'hello'", false},
		{"CURRENT_TIMESTAMP", false},
		{"bad(); DROP TABLE users; --", true},
		{"", true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			_, err := SafeDefaultLiteral(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("SafeDefaultLiteral(%q): err=%v, wantErr=%v", tc.input, err, tc.wantErr)
			}
		})
	}
}

// F-006 regression: all provider Quote() helpers must reject unsafe
// identifiers before they reach Exec. We exercise Oracle and SQL Server
// since those were the previously-unvalidated paths.
func TestProviderQuoteValidatesUnsafeIdents(t *testing.T) {
	unsafe := []string{
		"",                       // empty
		"foo\x00bar",             // NUL byte
		"../etc/passwd",          // path separator
		"foo/../bar",             // traversal
		strings.Repeat("a", 200), // too long
	}
	for _, dbType := range []string{"oracle", "sqlserver"} {
		p, err := NewProvider(dbType)
		if err != nil {
			t.Fatalf("NewProvider(%s): %v", dbType, err)
		}
		for _, in := range unsafe {
			got := p.Quote(in)
			// Both Oracle and SQL Server now route through SafePgIdent,
			// which rejects unsafe inputs by returning an empty quoted
			// string ("") rather than a constructed dangerous identifier.
			if got != "" {
				t.Errorf("%s.Quote(%q) = %q, want empty (rejected)", dbType, in, got)
			}
		}
	}
}
