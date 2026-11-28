//go:build server

package main

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/ys-ll/uniterm/backend/log"
	"github.com/ys-ll/uniterm/backend/serverpolicy"
)

// The operator policy is loaded exactly once, when the App service is
// registered. The Wails MethodFilter and the runtime guards (container
// transport, tenant path confinement) therefore always observe the same
// snapshot; changing UNITERM_SERVER_POLICY requires a worker restart.
var (
	serverPolicyOnce  sync.Once
	serverPolicyValue *serverpolicy.Policy
)

func currentServerPolicy() *serverpolicy.Policy {
	serverPolicyOnce.Do(func() {
		serverPolicyValue = serverpolicy.LoadFromEnv()
		if err := serverPolicyValue.LoadError(); err != nil {
			log.Writef("[serverpolicy] policy from %s failed closed: %v", serverPolicyValue.Source(), err)
			return
		}
		log.Writef("[serverpolicy] loaded from %s (iAcceptRisk=%v, workerDocker=%v)",
			serverPolicyValue.Source(), serverPolicyValue.RiskAccepted(), serverPolicyValue.WorkerDocker())
	})
	return serverPolicyValue
}

// appServiceOptions installs the default-deny App method filter in the
// multi-tenant server build.
func appServiceOptions() application.ServiceOptions {
	return application.ServiceOptions{MethodFilter: currentServerPolicy().Allowed}
}
