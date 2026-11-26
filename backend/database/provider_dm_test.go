package database

import (
	"testing"
)

// DSN shape, placeholder style and paging for the DM provider.

func TestDMDSN(t *testing.T) {
	p := &dmProvider{}
	if got, want := p.DriverName(), "dm"; got != want {
		t.Errorf("DriverName = %q, want %q", got, want)
	}
	if got, want := placeholder(p.DriverName(), 1), "?"; got != want {
		t.Errorf("placeholder = %q, want %q", got, want)
	}

	tests := []struct {
		name string
		port int
		want string
	}{
		{"explicit port", 5237, "dm://u:p@h:5237"},
		{"default port", 0, "dm://u:p@h:5236"},
	}
	for _, tt := range tests {
		if got := p.DSN("h", tt.port, "u", "p", "db", nil); got != tt.want {
			t.Errorf("%s: DSN = %q, want %q", tt.name, got, tt.want)
		}
	}

	// extraParams land in the query string in sorted order.
	if got, want := p.DSN("h", 0, "u", "p", "", map[string]string{"appName": "mtk", "logLevel": "all"}),
		"dm://u:p@h:5236?appName=mtk&logLevel=all"; got != want {
		t.Errorf("DSN with extraParams = %q, want %q", got, want)
	}
}

func TestDMQuote(t *testing.T) {
	p := &dmProvider{}
	if got, want := p.Quote(`na"me`), `"na""me"`; got != want {
		t.Errorf("Quote = %q, want %q", got, want)
	}
}

func TestDMTextCastAndPaging(t *testing.T) {
	if got, want := textCast("dm", `"C"`), `CAST("C" AS VARCHAR(4000))`; got != want {
		t.Errorf("textCast(dm) = %q, want %q", got, want)
	}
	p := &dmProvider{}
	if got, want := p.PagedTableQuery("", "t", 10, 20), `SELECT * FROM "t" LIMIT 10 OFFSET 20`; got != want {
		t.Errorf("PagedTableQuery = %q, want %q", got, want)
	}
	if got, want := p.DefaultTableQuery("", "t", 10, 0), `SELECT * FROM "t" LIMIT 10`; got != want {
		t.Errorf("DefaultTableQuery = %q, want %q", got, want)
	}
}
