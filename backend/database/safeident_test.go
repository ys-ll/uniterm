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