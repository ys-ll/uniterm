package container

import (
	"reflect"
	"testing"
)

func TestPSArgs(t *testing.T) {
	got := psArgs(RuntimeDocker, "")
	want := []string{"docker", "ps", "-a", "--format", "{{json .}}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestPSArgsNerdctlNamespace(t *testing.T) {
	got := psArgs(RuntimeNerdctl, "k8s.io")
	want := []string{"nerdctl", "--namespace", "k8s.io", "ps", "-a", "--format", "{{json .}}"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestExecArgs(t *testing.T) {
	got := execArgs(RuntimeDocker, "", "abc123", "sh")
	want := []string{"docker", "exec", "-it", "abc123", "sh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestJoinShellCommand(t *testing.T) {
	got := JoinShellCommand([]string{"docker", "exec", "-it", "my'container", "sh"})
	want := `docker exec -it 'my'\''container' sh`
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
