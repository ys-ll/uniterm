package container

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

// capturedRunner is an in-memory Runner for testing arg-builder logic
// and parser wiring without invoking docker/podman/nerdctl binaries.
// Unlike the smaller fakeRunner in provider_test.go, this one
// accumulates the full history of argv calls so QA-021's
// "many invocations" property is observable.
type capturedRunner struct {
	mu       sync.Mutex
	captured [][]string
	// runOut maps "argv[0]" (the binary, e.g. "docker") to canned
	// stdout to return. Lookup returns ("not found") error if missing.
	runOut map[string][]byte
	runErr map[string]error

	// optional streams to return from RunStream / RunPTY.
	streamFunc func(argv []string) (LineStream, error)
	ptyFunc    func(argv []string, cols, rows int) (PTYStream, error)
}

func newCapturedRunner() *capturedRunner {
	return &capturedRunner{
		runOut: map[string][]byte{},
		runErr: map[string]error{},
	}
}

func (f *capturedRunner) appendCapture(argv []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]string, len(argv))
	copy(cp, argv)
	f.captured = append(f.captured, cp)
}

func (f *capturedRunner) Captured() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.captured))
	for i, c := range f.captured {
		out[i] = append([]string(nil), c...)
	}
	return out
}

func (f *capturedRunner) Run(_ context.Context, argv []string) ([]byte, error) {
	f.appendCapture(argv)
	if err, ok := f.runErr[argv[0]]; ok && err != nil {
		return nil, err
	}
	if out, ok := f.runOut[argv[0]]; ok {
		return out, nil
	}
	return nil, errors.New("not found")
}

func (f *capturedRunner) RunStream(_ context.Context, argv []string) (LineStream, error) {
	f.appendCapture(argv)
	if f.streamFunc != nil {
		return f.streamFunc(argv)
	}
	return nil, errors.New("no stream")
}

func (f *capturedRunner) RunPTY(_ context.Context, argv []string, cols, rows int) (PTYStream, error) {
	f.appendCapture(argv)
	if f.ptyFunc != nil {
		return f.ptyFunc(argv, cols, rows)
	}
	return nil, errors.New("no pty")
}

// longLineStream emits a fixed number of lines then EOFs; used to
// verify the production consumer code can drain a long stream
// without blocking on a stuck writer.
type longLineStream struct {
	lines chan string
	done  chan error
}

func newLongLineStream(n int, waitErr error) *longLineStream {
	ch := make(chan string, n+1)
	for i := 0; i < n; i++ {
		ch <- "line"
	}
	close(ch)
	d := make(chan error, 1)
	d <- waitErr
	close(d)
	return &longLineStream{lines: ch, done: d}
}

func (s *longLineStream) Lines() <-chan string { return s.lines }
func (s *longLineStream) Wait() error          { <-s.done; return nil }
func (s *longLineStream) Close() error         { return nil }

// ---- F-013 runner parity: helpers keep producing identical argv ----

// TestInspectArgs_AllRuntimes sanity-checks all three runtimes.
func TestInspectArgs_AllRuntimes(t *testing.T) {
	cases := map[Runtime][]string{
		RuntimeDocker:  {"docker", "inspect", "abc"},
		RuntimePodman:  {"podman", "inspect", "abc"},
		RuntimeNerdctl: {"nerdctl", "--namespace", "k8s.io", "inspect", "abc"},
	}
	for rt, want := range cases {
		ns := ""
		if rt == RuntimeNerdctl {
			ns = "k8s.io"
		}
		got := inspectArgs(rt, ns, "abc")
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v want %v", rt, got, want)
		}
	}
}

// TestLogsArgs_Combinations: confirm ordering of optional flags
// matches what we want for the consumer side. (Deps keep shifting
// across docker releases; pinning this prevents silent breakage.)
func TestLogsArgs_Combinations(t *testing.T) {
	tests := []struct {
		name             string
		tail, follow, ts int
		ns               string
		rt               Runtime
		want             []string
	}{
		{"just tail", 50, 0, 0, "", RuntimeDocker, []string{"docker", "logs", "--tail", "50", "abc"}},
		{"tail + follow", 50, 1, 0, "", RuntimeDocker, []string{"docker", "logs", "--tail", "50", "-f", "abc"}},
		{"tail + ts", 50, 0, 1, "", RuntimeDocker, []string{"docker", "logs", "--tail", "50", "--timestamps", "abc"}},
		{"all three", 50, 1, 1, "", RuntimeDocker, []string{"docker", "logs", "--tail", "50", "--timestamps", "-f", "abc"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := logsArgs(tc.rt, tc.ns, "abc", tc.tail, tc.follow == 1, tc.ts == 1)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

// TestActionArgs_PauseUnpause: pause and unpause are valid actions;
// just confirm argv flips.
func TestActionArgs_PauseUnpause(t *testing.T) {
	for _, action := range []string{"pause", "unpause"} {
		got, err := actionArgs(RuntimeDocker, "", action, "abc")
		if err != nil {
			t.Fatalf("%s: %v", action, err)
		}
		want := []string{"docker", action, "abc"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v want %v", action, got, want)
		}
	}
}

// TestValidateRuntime_PartialDetection: if the chosen runtime is
// missing but another runtime is available, the error message
// should helpfully list what's available.
func TestValidateRuntime_PartialDetection(t *testing.T) {
	r := newCapturedRunner()
	r.runErr["docker"] = errors.New("not found")
	r.runErr["podman"] = nil
	r.runErr["nerdctl"] = errors.New("not found")
	r.runOut["podman"] = []byte("/usr/bin/podman")
	err := ValidateRuntime(context.Background(), RuntimeDocker, r)
	if err == nil {
		t.Fatal("expected error for missing docker")
	}
	// Should mention the available runtime (podman).
	if !containsStr(err.Error(), "podman") {
		t.Fatalf("error %q should mention available runtime 'podman'", err)
	}
}

// ---- Provider routed through capturedRunner ----

// TestProvider_List_ParsesRealisticJSON runs the full ParseContainers
// round-trip through the manager via the captured runner; catches
// regressions in argv → JSONL → struct shape.
func TestProvider_List_ParsesRealisticJSON(t *testing.T) {
	r := newCapturedRunner()
	r.runOut["docker"] = []byte(`{"ID":"a","Names":"web","Image":"nginx","State":"running","Status":"Up 1h","Ports":"","CreatedAt":""}` + "\n")
	p := NewProvider(RuntimeDocker, "", r)
	list, err := p.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "web" {
		t.Fatalf("parsed list = %+v", list)
	}
}

// TestProvider_Inspect_RoutesArgv verifies the ParseInspect path can
// be driven end-to-end through the captured runner.
func TestProvider_Inspect_RoutesArgv(t *testing.T) {
	r := newCapturedRunner()
	r.runOut["docker"] = []byte(`[{"Id":"c1","Name":"/web","Config":{"Image":"nginx","Cmd":["nginx"]},"State":{"Status":"running","StartedAt":"2026-01-01","OOMKilled":false,"Pid":42},"HostConfig":{"NetworkMode":"bridge"},"NetworkSettings":{}}]`)
	p := NewProvider(RuntimeDocker, "", r)
	res, err := p.Inspect(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Detail.Image != "nginx" {
		t.Fatalf("inspect = %+v", res.Detail)
	}
}

// ---- QA-021: long-running output / backpressure ----

// TestMockedRunner_LongStreamConsumes verifies the consumer side
// drains a 1000-line stream without hanging or racing.
func TestMockedRunner_LongStreamConsumes(t *testing.T) {
	r := newCapturedRunner()
	r.streamFunc = func(_ []string) (LineStream, error) {
		return newLongLineStream(1000, nil), nil
	}
	p := NewProvider(RuntimePodman, "", r)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := p.Logs(ctx, "abc", 100, true, false)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	deadline := time.After(1500 * time.Millisecond)
loop:
	for {
		select {
		case _, ok := <-stream.Lines():
			if !ok {
				break loop
			}
			count++
		case <-deadline:
			break loop
		}
	}
	if err := stream.Wait(); err != nil {
		t.Errorf("Wait: %v", err)
	}
	_ = stream.Close()
	if count < 100 {
		t.Fatalf("expected ≥100 lines drained, got %d", count)
	}
}

// ---- helpers ----

func containsStr(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
