package diag

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
)

var wrappedMu sync.Mutex

// Wrap runs fn and logs any error (or panic) under the given tag.
func Wrap(tag string, fn func() error) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			Error(tag, err.Error(), map[string]any{"stack": string(stack())})
		}
		if err != nil {
			Error(tag, err.Error(), nil)
		}
	}()
	return fn()
}

// Wrap1 is the generic single-result variant.
func Wrap1[A any](tag string, fn func() (A, error)) (result A, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
			Error(tag, err.Error(), map[string]any{"stack": string(stack())})
		}
		if err != nil {
			Error(tag, err.Error(), nil)
		}
	}()
	return fn()
}

// WrapAll scans a struct for exported methods whose final return is
// `error`, wrapping each with a logging shim. The current implementation
// is a no-op scan: it walks the type table to verify the conventions but
// does not replace the methods (Go method tables are immutable). Callers
// that want per-binding logging should wrap callsites manually with
// Wrap/Wrap1 or rely on the binding glue registered at app startup.
func WrapAll(app any) error {
	v := reflect.ValueOf(app)
	if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Struct {
		return fmt.Errorf("WrapAll: need *struct, got %T", app)
	}
	t := v.Elem().Type()
	wrappedMu.Lock()
	defer wrappedMu.Unlock()
	errType := reflect.TypeOf((*error)(nil)).Elem()
	for i := 0; i < t.NumMethod(); i++ {
		m := t.Method(i)
		sig := m.Type
		if sig.NumOut() == 0 {
			continue
		}
		if sig.Out(sig.NumOut()-1) != errType {
			continue
		}
		// Method is an error-returning one — eligible for wrap.
		_ = m
	}
	return nil
}

func stack() []byte { return []byte(stackString()) }

func stackString() string {
	var b strings.Builder
	pc := make([]uintptr, 32)
	n := runtime.Callers(2, pc)
	frames := runtime.CallersFrames(pc[:n])
	for {
		f, more := frames.Next()
		b.WriteString(f.File)
		b.WriteByte(':')
		b.WriteString(fmt.Sprintf("%d", f.Line))
		b.WriteByte(' ')
		b.WriteString(f.Function)
		b.WriteByte('\n')
		if !more {
			break
		}
	}
	return b.String()
}
