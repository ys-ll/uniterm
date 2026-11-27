package serverpolicy

import (
	"fmt"
	"strings"
)

// CheckContainerTransport enforces the worker-local container rule.
//
// The transport discriminator is cfg.ContainerTransport only. sshUser is an
// optional credential-completion field, never a transport signal: an SSH
// container connection with an empty sshUser is accepted exactly like one with
// a user, because the referenced SSH connection may already carry it.
//
//   - "local" (and any case variant) requires workerDocker + iAcceptRisk.
//   - "ssh" and the empty/default transport are accepted.
func CheckContainerTransport(policy *Policy, transport string) error {
	if !strings.EqualFold(strings.TrimSpace(transport), "local") {
		return nil
	}
	if policy == nil || !policy.WorkerDocker() {
		return fmt.Errorf("%w: the worker-local container transport requires workerDocker:true and iAcceptRisk:true in UNITERM_SERVER_POLICY", ErrDenied)
	}
	return nil
}
