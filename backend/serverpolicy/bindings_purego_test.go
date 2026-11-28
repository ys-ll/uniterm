//go:build purego

package serverpolicy

import (
	"context"
	"hash/fnv"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// policyBindingService is a fake App-shaped service. Every method on it is
// denied by the default policy, so Bindings.Add performs no registration and
// never needs the package-global *application.App for debug logging. (The
// headless Linux host has no GTK runtime, so constructing a real App is not
// possible here; the Wails fork's own TestBindingsMethodFilter covers
// registration of allowed methods with a real global app.)
type policyBindingService struct{}

func (s *policyBindingService) OpenFileDialog() string    { return "denied" }
func (s *policyBindingService) SetDataDir() string        { return "denied" }
func (s *policyBindingService) StartTelnetServer() string { return "denied" }
func (s *policyBindingService) ServiceStartup(context.Context, application.ServiceOptions) error {
	return nil
}

func policyBindingFQN(method string) string {
	return "github.com/ys-ll/uniterm/backend/serverpolicy.policyBindingService." + method
}

func policyBindingID(method string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(policyBindingFQN(method)))
	return h.Sum32()
}

// TestBindingsFilterRemovesDeniedMethods proves that the real policy filter,
// installed through ServiceOptions.MethodFilter, leaves a denied method out of
// both lookup paths, and that the filter receives bare Go names (not FQNs).
func TestBindingsFilterRemovesDeniedMethods(t *testing.T) {
	policy := Load(mapEnv(nil))
	seen := map[string]bool{}
	filter := func(name string) bool {
		seen[name] = true
		return policy.Allowed(name)
	}

	bindings := application.NewBindings(nil, nil)
	service := application.NewServiceWithOptions(&policyBindingService{}, application.ServiceOptions{
		Name:         "policy-binding-test",
		MethodFilter: filter,
	})
	if err := bindings.Add(service); err != nil {
		t.Fatalf("Add: %v", err)
	}

	for _, denied := range []string{"StartTelnetServer", "SetDataDir", "OpenFileDialog"} {
		fqn := policyBindingFQN(denied)
		if method := bindings.Get(&application.CallOptions{MethodName: fqn}); method != nil {
			t.Errorf("Get(%q) = %v, want nil for a policy-denied method", fqn, method.Name)
		}
		if method := bindings.GetByID(policyBindingID(denied)); method != nil {
			t.Errorf("GetByID(%q) = %v, want nil for a policy-denied method", denied, method.Name)
		}
		if !seen[denied] {
			t.Errorf("filter never saw bare method name %q", denied)
		}
		if seen[fqn] {
			t.Errorf("filter saw FQN %q, want bare names only", fqn)
		}
	}
}
