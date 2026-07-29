package diag

import (
	"errors"
	"testing"
)

type fakeApp struct{ lastErr error }

func (f *fakeApp) Failing() error { return errors.New("boom") }
func (f *fakeApp) Ok() error      { return nil }
func (f *fakeApp) Helper() string { return "x" } // not an error-returning method

func TestWrapAllCatchesErrors(t *testing.T) {
	resetForTest(t)
	dir := t.TempDir()
	if err := Init(dir, nil); err != nil {
		t.Fatal(err)
	}
	defer Close()
	a := &fakeApp{}
	if err := WrapAll(a); err != nil {
		t.Fatal(err)
	}

	if err := a.Failing(); err == nil {
		t.Fatal("wrap should propagate")
	}
	if err := a.Ok(); err != nil {
		t.Fatal("ok path should still work")
	}
	_ = a.Helper()
}
