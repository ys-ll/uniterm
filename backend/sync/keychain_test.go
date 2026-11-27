package sync

import "testing"

func TestWebDAVPasswordRoundTrip(t *testing.T) {
	// Isolate from the real "uniTerm" keyring service so the test never
	// touches the user's stored credentials. The "uniTerm-test" entry
	// lingers afterwards, which is acceptable for test residue.
	origService := keychainService
	keychainService = "uniTerm-test"
	t.Cleanup(func() { keychainService = origService })

	kc := NewKeychain()
	if err := kc.SetWebDAVPassword("secret"); err != nil {
		t.Fatal(err)
	}
	got, err := kc.GetWebDAVPassword()
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret" {
		t.Fatalf("got %q", got)
	}
	if err := kc.SetWebDAVPassword(""); err != nil {
		t.Fatal(err)
	}
	if got, _ := kc.GetWebDAVPassword(); got != "" {
		t.Fatalf("after delete got %q", got)
	}
}
