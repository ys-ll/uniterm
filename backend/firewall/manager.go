package firewall

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// Manager owns the lifecycle of firewall operations against remote hosts. It
// detects which backend (firewalld/ufw) a host has and dispatches every
// operation to the matching adapter, so the frontend only deals with one
// unified model.
type Manager struct {
	mu      sync.Mutex
	emitter Emitter
}

func NewManager() *Manager {
	return &Manager{}
}

// SetEventEmitter wires the manager to the frontend event bus.
func (m *Manager) SetEventEmitter(fn Emitter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.emitter = fn
}

func (m *Manager) emit(name string, payload any) {
	m.mu.Lock()
	e := m.emitter
	m.mu.Unlock()
	if e != nil {
		e(name, payload)
	}
}

// ErrNotInstalled is returned when no supported firewall backend is present on
// the target host assisted by status queries.
var ErrNotInstalled = fmt.Errorf("目标主机未安装支持的防火墙 (firewalld / ufw)")

// notInstalled reports whether the detect error means no backend was found.
func notInstalled(backends []BackendKind) bool {
	for _, b := range backends {
		if b != BackendNone {
			return false
		}
	}
	return true
}

// Probe connects to the remote host and detects which firewall backend is
// present and whether it is active.
func (m *Manager) Probe(r *ResolvedSSH) (*Status, error) {
	active := false
	backendKind := BackendNone

	// ufw and firewalld can coexist (e.g. firewalld installed but disabled on
	// Debian). Prefer whichever is actually active; fall back to the first that
	// is merely installed.
	installed := make([]BackendKind, 0, 2)

	if ok, act, err := firewalldProbe(r); err == nil && ok {
		installed = append(installed, BackendFirewalld)
		if act {
			backendKind = BackendFirewalld
			active = true
		}
	}
	if ok, act, err := ufwProbe(r); err == nil && ok {
		installed = append(installed, BackendUfw)
		if act {
			backendKind = BackendUfw
			active = true
		}
	}

	if backendKind == BackendNone && len(installed) > 0 {
		// none active, but one is installed → report that one, inactive.
		backendKind = installed[0]
	}

	status := &Status{Backend: backendKind, Active: active}

	if backendKind == BackendFirewalld {
		status.DefaultPolicy = firewalldDefaultPolicy(r)
		status.AutoStart = firewalldAutoStart(r)
	} else if backendKind == BackendUfw {
		status.DefaultPolicy = ufwDefaultPolicy(r)
		status.AutoStart = ufwAutoStart(r)
	}

	if notInstalled([]BackendKind{backendKind}) {
		return status, ErrNotInstalled
	}
	return status, nil
}

// SetEnabled turns the detected firewall on or off. Enabling is guarded by the
// same SSH check as applying rules because it can also activate a lockout.
func (m *Manager) SetEnabled(r *ResolvedSSH, enable bool) error {
	status, err := m.Probe(r)
	if err != nil {
		return err
	}
	if enable {
		rules, err := listRules(r, status.Backend)
		if err != nil {
			return err
		}
		if err := requireSSHAccess(r, rules); err != nil {
			return err
		}
	}
	switch status.Backend {
	case BackendFirewalld:
		return firewalldSetEnabled(r, enable)
	case BackendUfw:
		return ufwSetEnabled(r, enable)
	default:
		return ErrNotInstalled
	}
}

// SetAutoStart toggles whether the detected firewall starts on boot
// (systemctl enable/disable). This is orthogonal to whether it is running right
// now and does not alter current rules, so no SSH lockout check is performed.
func (m *Manager) SetAutoStart(r *ResolvedSSH, enable bool) error {
	status, err := m.Probe(r)
	if err != nil {
		return err
	}
	switch status.Backend {
	case BackendFirewalld:
		return firewalldSetAutoStart(r, enable)
	case BackendUfw:
		return ufwSetAutoStart(r, enable)
	default:
		return ErrNotInstalled
	}
}

// SaveConfig reconciles the persistent port rules without activating them.
// Additions are written before removals so a save cannot temporarily cut off
// access to a service that is present in both configurations.
func (m *Manager) SaveConfig(r *ResolvedSSH, desired []PortRule) error {
	status, err := m.Probe(r)
	if err != nil {
		return err
	}
	current, err := listRules(r, status.Backend)
	if err != nil {
		return err
	}

	currentSet := make(map[string]PortRule, len(current))
	for _, rule := range current {
		currentSet[ruleKey(rule)] = rule
	}
	desiredSet := make(map[string]PortRule, len(desired))
	for _, rule := range desired {
		rule.Port = strings.TrimSpace(rule.Port)
		rule.Source = strings.TrimSpace(rule.Source)
		rule.Persist = PersistPermanent
		if rule.Direction == "" {
			rule.Direction = DirectionIn
		}
		if rule.Protocol == "" {
			rule.Protocol = ProtocolTCP
		}
		desiredSet[ruleKey(rule)] = rule
	}

	for _, rule := range desiredSet {
		if _, exists := currentSet[ruleKey(rule)]; exists {
			continue
		}
		if err := saveRule(r, status.Backend, rule, ActionAllow); err != nil {
			return fmt.Errorf("保存端口 %s/%s 失败: %w", rule.Port, rule.Protocol, err)
		}
	}
	removed := make(map[string]bool)
	for _, rule := range current {
		key := ruleKey(rule)
		if _, exists := desiredSet[key]; exists || removed[key] {
			continue
		}
		if err := saveRule(r, status.Backend, rule, ActionDeny); err != nil {
			return fmt.Errorf("移除端口 %s/%s 失败: %w", rule.Port, rule.Protocol, err)
		}
		removed[key] = true
	}
	return nil
}

// ApplyConfig activates the saved configuration only after confirming that the
// permanent rules allow both port 22 and the port used by this SSH connection.
func (m *Manager) ApplyConfig(r *ResolvedSSH) error {
	status, err := m.Probe(r)
	if err != nil {
		return err
	}
	rules, err := listRules(r, status.Backend)
	if err != nil {
		return err
	}
	if err := requireSSHAccess(r, rules); err != nil {
		return err
	}
	switch status.Backend {
	case BackendFirewalld:
		return firewalldReload(r)
	case BackendUfw:
		_, err := runCommand(r, "ufw reload")
		return err
	default:
		return ErrNotInstalled
	}
}

// ListRules returns the persistent allow port rules for editing.
func (m *Manager) ListRules(r *ResolvedSSH) ([]PortRule, error) {
	status, err := m.Probe(r)
	if err != nil {
		return nil, err
	}
	return listRules(r, status.Backend)
}

func listRules(r *ResolvedSSH, backend BackendKind) ([]PortRule, error) {
	switch backend {
	case BackendFirewalld:
		return firewalldListRules(r)
	case BackendUfw:
		return ufwListRules(r)
	default:
		return nil, ErrNotInstalled
	}
}

func saveRule(r *ResolvedSSH, backend BackendKind, rule PortRule, action Action) error {
	switch backend {
	case BackendFirewalld:
		return firewalldSaveRule(r, rule, action)
	case BackendUfw:
		return ufwSaveRule(r, rule, action)
	default:
		return ErrNotInstalled
	}
}

func ruleKey(rule PortRule) string {
	return strings.Join([]string{
		string(rule.Direction),
		string(rule.Protocol),
		strings.TrimSpace(rule.Port),
		strings.ToLower(strings.TrimSpace(rule.Source)),
	}, "|")
}

func requireSSHAccess(r *ResolvedSSH, rules []PortRule) error {
	ports := []int{22}
	if r != nil && r.Port > 0 && r.Port != 22 {
		ports = append(ports, r.Port)
	}
	for _, port := range ports {
		if !sshPortAllowed(rules, port) {
			return fmt.Errorf("已取消应用：永久配置中未放行 %d/tcp 入站连接（来源不限）。请先添加并保存该规则，避免 SSH 自锁失联", port)
		}
	}
	return nil
}

func sshPortAllowed(rules []PortRule, port int) bool {
	for _, rule := range rules {
		if rule.Direction != DirectionIn || rule.Protocol != ProtocolTCP || !portRangeContains(rule.Port, port) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(rule.Source)) {
		case "", "any", "anywhere", "0.0.0.0/0", "::/0":
			return true
		}
	}
	return false
}

func portRangeContains(value string, port int) bool {
	parts := strings.SplitN(strings.TrimSpace(value), "-", 2)
	start, err := strconv.Atoi(parts[0])
	if err != nil || start < 1 || start > 65535 {
		return false
	}
	if len(parts) == 1 {
		return start == port
	}
	end, err := strconv.Atoi(parts[1])
	return err == nil && end >= start && end <= 65535 && port >= start && port <= end
}
