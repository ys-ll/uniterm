// Package serverpolicy implements the operator-controlled method policy for
// the multi-tenant server build.
//
// The server build installs application.ServiceOptions.MethodFilter backed by
// Policy.Allowed, so a method that is not allowed is never registered in the
// Wails bindings registry: it cannot be called by name or by generated ID.
//
// Policy input is environment-only. UNITERM_SERVER_POLICY carries an inline
// JSON object; UNITERM_SERVER_POLICY_FILE names a file and wins when set. A
// missing/malformed file, malformed JSON, an unknown key, or a non-object
// document fails closed: every optional group stays denied and LoadError
// reports why. Optional groups additionally require iAcceptRisk:true.
package serverpolicy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ErrDenied is the sentinel wrapped by every runtime policy denial, so callers
// can present one clear "blocked by server policy" message.
var ErrDenied = errors.New("blocked by server policy")

// Policy is an immutable snapshot of the operator policy.
//
// It is loaded once at service-registration time. Mutating the environment
// afterwards has no effect until the process restarts, which keeps the bound
// method set and the runtime guards in agreement (no stale half-reloaded
// policy).
type Policy struct {
	groups   map[Group]bool
	risk     bool
	allowAll bool
	loadErr  error
	source   string
}

// AllowAll returns the desktop policy: every known method is allowed. The
// desktop build does not install a MethodFilter, so this only affects the
// shared container transport guard, which must not block desktop use.
func AllowAll() *Policy {
	return &Policy{groups: map[Group]bool{}, risk: true, allowAll: true, source: "desktop"}
}

// Defaults returns the secure default policy: every group listed in
// OptionalGroups is denied, and only the always-allowed groups are bound.
func Defaults() *Policy {
	return &Policy{groups: map[Group]bool{}, source: "defaults"}
}

// LoadFromEnv loads the policy from UNITERM_SERVER_POLICY and
// UNITERM_SERVER_POLICY_FILE.
func LoadFromEnv() *Policy {
	return Load(os.Getenv)
}

// Load loads the policy through getenv so tests can inject a deterministic
// environment.
func Load(getenv func(string) string) *Policy {
	if getenv == nil {
		return failClosed("defaults", errors.New("nil environment getter"))
	}

	if file := strings.TrimSpace(getenv("UNITERM_SERVER_POLICY_FILE")); file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return failClosed("file:"+file, fmt.Errorf("read server policy file: %w", err))
		}
		enabled, risk, err := parse(data)
		if err != nil {
			return failClosed("file:"+file, err)
		}
		return &Policy{groups: enabled, risk: risk, source: "file:" + file}
	}

	raw := strings.TrimSpace(getenv("UNITERM_SERVER_POLICY"))
	if raw == "" {
		return Defaults()
	}
	enabled, risk, err := parse([]byte(raw))
	if err != nil {
		return failClosed("env", err)
	}
	return &Policy{groups: enabled, risk: risk, source: "env"}
}

func failClosed(source string, err error) *Policy {
	return &Policy{groups: map[Group]bool{}, source: source, loadErr: err}
}

func parse(data []byte) (map[Group]bool, bool, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, false, errors.New("policy is empty")
	}
	if trimmed[0] != '{' {
		return nil, false, errors.New("policy must be a JSON object")
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, false, fmt.Errorf("parse policy JSON: %w", err)
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		return nil, false, errors.New("trailing data after the policy object")
	}

	enabled := make(map[Group]bool)
	risk := false
	for key, value := range raw {
		if key == "iAcceptRisk" {
			if err := json.Unmarshal(value, &risk); err != nil {
				return nil, false, fmt.Errorf("iAcceptRisk must be a boolean: %w", err)
			}
			continue
		}
		group := Group(key)
		if !isOptionalGroup(group) {
			return nil, false, fmt.Errorf("unknown or non-configurable policy key %q", key)
		}
		var on bool
		if err := json.Unmarshal(value, &on); err != nil {
			return nil, false, fmt.Errorf("policy key %q must be a boolean: %w", key, err)
		}
		enabled[group] = on
	}
	return enabled, risk, nil
}

func isOptionalGroup(group Group) bool {
	for _, optional := range OptionalGroups {
		if optional == group {
			return true
		}
	}
	return false
}

// Allowed reports whether the bare Go method name may be bound. Unknown names
// are always denied: there is no implicit "remote" default.
func (p *Policy) Allowed(method string) bool {
	if p == nil {
		return false
	}
	group, ok := Methods[method]
	if !ok {
		return false
	}
	return p.Enabled(group)
}

// Enabled reports whether a group is enabled.
func (p *Policy) Enabled(group Group) bool {
	if p == nil {
		return false
	}
	if p.allowAll {
		return true
	}
	if AlwaysAllowedGroups[group] {
		return true
	}
	if PermanentDeniedGroups[group] {
		return false
	}
	return p.risk && p.groups[group]
}

// WorkerDocker reports whether the operator enabled the worker-local
// container engine with iAcceptRisk.
func (p *Policy) WorkerDocker() bool {
	return p.Enabled(GroupWorkerDocker)
}

// LoadError reports the fail-closed reason, if any.
func (p *Policy) LoadError() error {
	if p == nil {
		return errors.New("server policy is nil")
	}
	return p.loadErr
}

// Source describes where the policy came from (env, file:<path>, defaults,
// desktop).
func (p *Policy) Source() string {
	if p == nil {
		return "nil"
	}
	return p.source
}

// RiskAccepted reports whether iAcceptRisk was set.
func (p *Policy) RiskAccepted() bool {
	if p == nil {
		return false
	}
	return p.risk
}
