package container

import (
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// CommitOptions creates an image from the container's writable layer and
// configuration. Docker's default pause behavior is intentionally preserved.
type CommitOptions struct {
	Image   string `json:"image"`
	Author  string `json:"author"`
	Message string `json:"message"`
}

func (p *Provider) LoadImage(ctx context.Context, archive io.Reader) (string, error) {
	if p.rt != RuntimeDocker {
		return "", fmt.Errorf("image archive loading is currently supported only for Docker")
	}
	if archive == nil {
		return "", fmt.Errorf("image archive required")
	}
	out, err := p.runner.RunStdinStream(ctx, []string{"docker", "load"}, archive)
	return strings.TrimSpace(string(out)), err
}

// imageReferenceRE approximates Docker's reference grammar (domain[:port]/path[:tag][@digest]):
// path components are lowercase alphanumerics separated by . _ -. The optional
// leading registry component must contain a dot or a port — mirroring Docker's
// splitDockerDomain rule where a bare first segment like "UPPER/repo" is
// treated as part of the (lowercase) path, not a host. The tag follows
// Docker's [\w][\w.-]{0,127} rule; digests use the generic algorithm:hex form
// rather than enumerating every hash algorithm.
var imageReferenceRE = regexp.MustCompile(`^(?:[a-zA-Z0-9-]+(?:(?:\.[a-zA-Z0-9.-]+)+(?::[0-9]+)?|:[0-9]+)/)?(?:[a-z0-9]+(?:[._-][a-z0-9]+)*)(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*(?::[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127})?(?:@[a-z0-9]+(?:[.+_-][a-z0-9]+)*:[a-fA-F0-9]{32,})?$`)

// validImageReference rejects names Docker would refuse anyway (empty, "..",
// uppercase repository, trailing slash, double slash), so the user gets the
// error in uniTerm instead of parsing docker's output.
func validImageReference(ref string) bool {
	return len(ref) <= 255 && imageReferenceRE.MatchString(ref)
}

func (p *Provider) CommitImage(ctx context.Context, id string, opts CommitOptions) (string, error) {
	if p.rt != RuntimeDocker {
		return "", fmt.Errorf("container image commit is currently supported only for Docker")
	}
	id, opts.Image = strings.TrimSpace(id), strings.TrimSpace(opts.Image)
	if id == "" || strings.HasPrefix(id, "-") {
		return "", fmt.Errorf("container ID required")
	}
	if !validImageReference(opts.Image) {
		return "", fmt.Errorf("invalid target image name %q (expected repository[:tag])", opts.Image)
	}
	argv := []string{"docker", "commit"}
	if opts.Author != "" {
		argv = append(argv, "--author", opts.Author)
	}
	if opts.Message != "" {
		argv = append(argv, "--message", opts.Message)
	}
	argv = append(argv, id, opts.Image)
	out, err := p.runner.Run(ctx, argv)
	return strings.TrimSpace(string(out)), err
}

type imageOperation struct {
	connID   string
	provider *Provider
	ctx      context.Context
	cancel   context.CancelFunc
	running  bool
}

// PrepareImageOperation registers cancellation before any work starts, so
// Cancel can also win the race with the subsequent load/commit RPC.
func (m *Manager) PrepareImageOperation(connID string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.conns[connID]
	if !ok {
		return "", fmt.Errorf("container connection not found: %s", connID)
	}
	if c.provider.Runtime() != RuntimeDocker {
		return "", fmt.Errorf("image loading and commit are currently supported only for Docker")
	}
	ctx, cancel := context.WithCancel(context.Background())
	id := uuid.NewString()
	m.imageOps[id] = &imageOperation{connID: connID, provider: c.provider, ctx: ctx, cancel: cancel}
	return id, nil
}

func (m *Manager) CancelImageOperation(id string) {
	m.mu.Lock()
	op := m.imageOps[id]
	delete(m.imageOps, id)
	m.mu.Unlock()
	if op != nil {
		op.cancel()
	}
}

func (m *Manager) runImageOperation(id string, run func(context.Context, *Provider) (string, error)) (string, error) {
	m.mu.Lock()
	op := m.imageOps[id]
	if op == nil || op.running {
		m.mu.Unlock()
		return "", fmt.Errorf("image operation is no longer available")
	}
	op.running = true
	m.mu.Unlock()
	defer m.CancelImageOperation(id)
	if err := op.ctx.Err(); err != nil {
		return "", err
	}
	out, err := run(op.ctx, op.provider)
	if op.ctx.Err() != nil {
		return "", op.ctx.Err()
	}
	return out, err
}

// LoadImageFile reads the archive on the app host and streams it through the
// selected connection. A remote Docker host never needs the local file path.
func (m *Manager) LoadImageFile(operationID, archivePath string) (string, error) {
	return m.runImageOperation(operationID, func(ctx context.Context, p *Provider) (string, error) {
		info, err := os.Stat(archivePath)
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("image archive must be a regular file")
		}
		file, err := os.Open(archivePath)
		if err != nil {
			return "", err
		}
		defer file.Close()
		return p.LoadImage(ctx, file)
	})
}

func (m *Manager) CommitImage(operationID, id string, opts CommitOptions) (string, error) {
	return m.runImageOperation(operationID, func(ctx context.Context, p *Provider) (string, error) {
		return p.CommitImage(ctx, id, opts)
	})
}
