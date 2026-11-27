package serverpolicy

import (
	"errors"
	"testing"
)

func TestCheckContainerTransport(t *testing.T) {
	defaults := Load(mapEnv(nil))
	if err := CheckContainerTransport(defaults, "local"); !errors.Is(err, ErrDenied) {
		t.Fatalf("local transport error = %v, want ErrDenied", err)
	}
	if err := CheckContainerTransport(defaults, " LOCAL "); !errors.Is(err, ErrDenied) {
		t.Fatalf("case-insensitive local transport error = %v, want ErrDenied", err)
	}
	for _, transport := range []string{"", "ssh", "SSH"} {
		if err := CheckContainerTransport(defaults, transport); err != nil {
			t.Fatalf("transport %q must be accepted on the SSH/default path: %v", transport, err)
		}
	}

	enabled := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY": `{"workerDocker":true,"iAcceptRisk":true}`,
	}))
	if err := CheckContainerTransport(enabled, "local"); err != nil {
		t.Fatalf("operator-enabled local transport was rejected: %v", err)
	}

	noRisk := Load(mapEnv(map[string]string{
		"UNITERM_SERVER_POLICY": `{"workerDocker":true}`,
	}))
	if err := CheckContainerTransport(noRisk, "local"); !errors.Is(err, ErrDenied) {
		t.Fatalf("local transport without risk acceptance = %v, want ErrDenied", err)
	}

	if err := CheckContainerTransport(nil, "local"); !errors.Is(err, ErrDenied) {
		t.Fatalf("nil policy local transport = %v, want ErrDenied", err)
	}
	if err := CheckContainerTransport(AllowAll(), "local"); err != nil {
		t.Fatalf("desktop AllowAll must accept the local transport: %v", err)
	}
}
