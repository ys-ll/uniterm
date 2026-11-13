package commandlimit

import "testing"

func TestNormalizeConfiguration(t *testing.T) {
	for _, tc := range []struct{ input, want int }{
		{0, 8}, {-1, 8}, {1, 1}, {16, 16}, {64, 64}, {1000, 64},
	} {
		if got := Normalize(tc.input); got != tc.want {
			t.Errorf("Normalize(%d)=%d; want %d", tc.input, got, tc.want)
		}
	}
}
