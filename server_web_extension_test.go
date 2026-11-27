//go:build server

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCredentialsAdminRoutesRequireGatewayAdminToken proves the tenant-facing
// worker token alone cannot reach the permanently denied credential-admin
// methods. The gateway's tenant proxy strips any client admin header and only
// its own postToWorker sets the admin token.
func TestCredentialsAdminRoutesRequireGatewayAdminToken(t *testing.T) {
	t.Setenv("UNITERM_WORKER_TOKEN", "tenant-token")
	t.Setenv("UNITERM_WORKER_ADMIN_TOKEN", "admin-token")

	svc := &WebStreamService{unitermApp: &App{}}
	for _, path := range []string{"/api/web/credentials/reset", "/api/web/credentials/rekey"} {
		cases := []struct {
			name       string
			adminToken string
			wantForbid bool
		}{
			{name: "no admin token", adminToken: "", wantForbid: true},
			{name: "wrong admin token", adminToken: "not-the-admin-token", wantForbid: true},
			{name: "gateway admin token", adminToken: "admin-token", wantForbid: false},
		}
		for _, tc := range cases {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("X-Uniterm-Worker-Token", "tenant-token")
				if tc.adminToken != "" {
					req.Header.Set(workerAdminHeader, tc.adminToken)
				}
				rec := httptest.NewRecorder()
				svc.ServeHTTP(rec, req)

				forbidden := rec.Code == http.StatusForbidden
				if forbidden != tc.wantForbid {
					t.Fatalf("status = %d, wantForbid=%v body=%s", rec.Code, tc.wantForbid, rec.Body.String())
				}
				if tc.wantForbid && !strings.Contains(rec.Body.String(), "admin credential routes") {
					t.Fatalf("forbidden body = %q, want a clear admin-route error", rec.Body.String())
				}
			})
		}
	}
}

func TestCredentialsAdminRoutesDisabledWithoutAdminToken(t *testing.T) {
	t.Setenv("UNITERM_WORKER_TOKEN", "")
	t.Setenv("UNITERM_WORKER_ADMIN_TOKEN", "")

	svc := &WebStreamService{unitermApp: &App{}}
	req := httptest.NewRequest(http.MethodPost, "/api/web/credentials/reset", strings.NewReader("{}"))
	rec := httptest.NewRecorder()
	svc.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 when no admin token is configured", rec.Code)
	}
}
