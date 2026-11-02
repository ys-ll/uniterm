package container

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type imageTestRunner struct {
	fakeRunner
	stream func(context.Context, []string, io.Reader) ([]byte, error)
}

func (r *imageTestRunner) RunStdinStream(ctx context.Context, args []string, input io.Reader) ([]byte, error) {
	r.lastArgv = args
	if r.stream != nil {
		return r.stream(ctx, args, input)
	}
	return nil, nil
}
func imageTestManager(r Runner) *Manager {
	m := NewManager()
	m.conns["docker"] = &conn{id: "docker", provider: NewProvider(RuntimeDocker, "", r)}
	return m
}

func TestLoadImageArchiveStreamsUnchanged(t *testing.T) {
	// Include binary data, not just text; Docker handles archive decompression.
	content := strings.Repeat("\x00\x1f\x8barchive\xff", 10000)
	path := filepath.Join(t.TempDir(), "image archive.tar.gz")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	r := &imageTestRunner{}
	r.stream = func(_ context.Context, args []string, input io.Reader) ([]byte, error) {
		if _, ok := input.(*os.File); !ok {
			t.Fatalf("expected streaming file, got %T", input)
		}
		got, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}
		if string(got) != content {
			t.Fatal("archive bytes changed")
		}
		if !reflect.DeepEqual(args, []string{"docker", "load"}) {
			t.Fatalf("args: %v", args)
		}
		return []byte("Loaded image: example:latest\n"), nil
	}
	m := imageTestManager(r)
	id, err := m.PrepareImageOperation("docker")
	if err != nil {
		t.Fatal(err)
	}
	out, err := m.LoadImageFile(id, path)
	if err != nil || out != "Loaded image: example:latest" {
		t.Fatalf("result: %q, %v", out, err)
	}
	if len(m.imageOps) != 0 {
		t.Fatal("completed operation leaked")
	}
}

func TestImageArchiveRejectsNonFilesAndSurfacesLoadErrors(t *testing.T) {
	sentinel := errors.New("archive is not a Docker image archive")
	r := &imageTestRunner{stream: func(context.Context, []string, io.Reader) ([]byte, error) { return nil, sentinel }}
	m := imageTestManager(r)
	for _, path := range []string{t.TempDir(), filepath.Join(t.TempDir(), "missing.tar")} {
		id, _ := m.PrepareImageOperation("docker")
		if _, err := m.LoadImageFile(id, path); err == nil {
			t.Fatal("expected invalid file error")
		}
		if r.lastArgv != nil {
			t.Fatal("invalid file reached Docker")
		}
	}
	path := filepath.Join(t.TempDir(), "invalid.tar")
	_ = os.WriteFile(path, []byte("invalid archive"), 0600)
	id, _ := m.PrepareImageOperation("docker")
	if _, err := m.LoadImageFile(id, path); !errors.Is(err, sentinel) {
		t.Fatalf("lost load error: %v", err)
	}
	if len(m.imageOps) != 0 {
		t.Fatal("failed operation leaked")
	}
}

func TestCommitImageOptionsAndValidation(t *testing.T) {
	r := &fakeRunner{runOut: []byte("sha256:new-image\n")}
	p := NewProvider(RuntimeDocker, "", r)
	opts := CommitOptions{Image: "registry:5000/team/image:v1", Author: "A Person", Message: "snapshot 'quoted' $(literal)"}
	out, err := p.CommitImage(context.Background(), "container-id", opts)
	if err != nil || out != "sha256:new-image" {
		t.Fatalf("result: %q, %v", out, err)
	}
	want := []string{"docker", "commit", "--author", opts.Author, "--message", opts.Message, "container-id", opts.Image}
	if !reflect.DeepEqual(r.lastArgv, want) {
		t.Fatalf("args: %v", r.lastArgv)
	}
	// Docker's default pause behavior must remain enabled.
	if strings.Contains(strings.Join(r.lastArgv, " "), "--pause") {
		t.Fatal("pause behavior overridden")
	}
	for _, image := range []string{"", " ", "--help", "bad name", "repo//x", "UPPER/repo", "repo:", "repo/", "repo..tag", "team/image:v1:extra"} {
		r.lastArgv = nil
		if _, err := p.CommitImage(context.Background(), "container-id", CommitOptions{Image: image}); err == nil {
			t.Fatal("invalid image accepted")
		}
		if r.lastArgv != nil {
			t.Fatal("invalid image reached runner")
		}
	}
	if _, err := p.CommitImage(context.Background(), "--help", opts); err == nil {
		t.Fatal("option accepted as container ID")
	}
}

func TestValidImageReference(t *testing.T) {
	// Valid references must pass the same gate, including registry with port,
	// digests, and uppercase tags (Docker allows mixed-case tags).
	for _, image := range []string{"nginx", "nginx:latest", "registry:5000/team/image:v1", "my-repo/my.image_x:V1.2", "alpine@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"} {
		if !validImageReference(image) {
			t.Errorf("validImageReference(%q) = false, want true", image)
		}
	}
	for _, image := range []string{"", "nginx:", "nginx@sha256:short", "my repo:v1", strings.Repeat("a", 256)} {
		if validImageReference(image) {
			t.Errorf("validImageReference(%q) = true, want false", image)
		}
	}
}

func TestImageOperationsRuntimeGuards(t *testing.T) {
	for _, rt := range []Runtime{RuntimePodman, RuntimeNerdctl, RuntimeWSLC} {
		r := &fakeRunner{}
		p := NewProvider(rt, "", r)
		if _, err := p.LoadImage(context.Background(), strings.NewReader("archive")); err == nil {
			t.Fatal("unsupported load accepted")
		}
		if _, err := p.CommitImage(context.Background(), "id", CommitOptions{Image: "image:tag"}); err == nil {
			t.Fatal("unsupported commit accepted")
		}
		m := NewManager()
		m.conns["other"] = &conn{provider: p}
		if _, err := m.PrepareImageOperation("other"); err == nil {
			t.Fatal("unsupported runtime prepared")
		}
		if r.lastArgv != nil {
			t.Fatal("unsupported operation reached runner")
		}
	}
}

func TestImageOperationCancelBeforeStart(t *testing.T) {
	r := &fakeRunner{}
	m := imageTestManager(r)
	id, _ := m.PrepareImageOperation("docker")
	m.CancelImageOperation(id)
	if _, err := m.CommitImage(id, "container", CommitOptions{Image: "image:tag"}); err == nil {
		t.Fatal("cancelled operation executed")
	}
	if r.lastArgv != nil {
		t.Fatal("cancelled operation reached runner")
	}
}

func TestImageLoadCancellationAndDisconnect(t *testing.T) {
	for _, disconnect := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "disconnect"}[disconnect], func(t *testing.T) {
			started := make(chan struct{})
			r := &imageTestRunner{stream: func(ctx context.Context, _ []string, _ io.Reader) ([]byte, error) {
				close(started)
				<-ctx.Done()
				return nil, ctx.Err()
			}}
			m := imageTestManager(r)
			path := filepath.Join(t.TempDir(), "image.tar")
			_ = os.WriteFile(path, []byte("archive"), 0600)
			id, _ := m.PrepareImageOperation("docker")
			result := make(chan error, 1)
			go func() { _, err := m.LoadImageFile(id, path); result <- err }()
			select {
			case <-started:
			case <-time.After(2 * time.Second):
				t.Fatal("load did not start")
			}
			if _, err := m.CommitImage(id, "container", CommitOptions{Image: "image:tag"}); err == nil {
				t.Fatal("operation reused while running")
			}
			if disconnect {
				m.Disconnect("docker")
			} else {
				m.CancelImageOperation(id)
			}
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("cancel result: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cancellation did not complete")
			}
			if len(m.imageOps) != 0 {
				t.Fatal("operation leaked")
			}
		})
	}
}
