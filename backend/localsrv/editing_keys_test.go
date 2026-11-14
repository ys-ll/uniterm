package localsrv

import (
	"bytes"
	"testing"
)

func TestNormalizeEditingKeys(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
		want []byte
	}{
		{
			name: "delete key ESC[3~ maps to 0x7F",
			in:   []byte{0x1B, '[', '3', '~'},
			want: []byte{0x7F},
		},
		{
			name: "backspace 0x08 maps to 0x7F",
			in:   []byte{0x08},
			want: []byte{0x7F},
		},
		{
			name: "plain typing passes through",
			in:   []byte("dir "),
			want: []byte("dir "),
		},
		{
			name: "mixed typing backspace and delete",
			in:   []byte{'d', 'i', 'r', 0x08, 0x1B, '[', '3', '~'},
			want: []byte{'d', 'i', 'r', 0x7F, 0x7F},
		},
		{
			name: "left arrow ESC[D passes through",
			in:   []byte{0x1B, '[', 'D'},
			want: []byte{0x1B, '[', 'D'},
		},
		{
			name: "literal ESC not part of del key passes through",
			in:   []byte{0x1B},
			want: []byte{0x1B},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := normalizeEditingKeys(c.in)
			if !bytes.Equal(got, c.want) {
				t.Fatalf("normalizeEditingKeys(% X) = % X, want % X", c.in, got, c.want)
			}
		})
	}
}
