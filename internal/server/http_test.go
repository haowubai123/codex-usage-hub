package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerRoutesHealthAndDashboard(t *testing.T) {
	srv := newTestServer(t)
	handler := srv.Handler()

	healthRec := httptest.NewRecorder()
	handler.ServeHTTP(healthRec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if healthRec.Code != http.StatusOK {
		t.Fatalf("health status = %d, body = %s", healthRec.Code, healthRec.Body.String())
	}

	dashboardRec := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRec, httptest.NewRequest(http.MethodGet, "/", nil))
	if dashboardRec.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, body = %s", dashboardRec.Code, dashboardRec.Body.String())
	}
	if !strings.Contains(dashboardRec.Body.String(), "Codex Usage") {
		t.Fatalf("dashboard body does not contain title: %s", dashboardRec.Body.String())
	}
}

func TestHandlerRoutesAPIsAndMethods(t *testing.T) {
	srv := newQueryTestServer(t)
	handler := srv.Handler()

	cases := []struct {
		method string
		path   string
		status int
	}{
		{method: http.MethodPost, path: "/api/v1/ingest", status: http.StatusUnauthorized},
		{method: http.MethodGet, path: "/api/v1/summary", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/breakdown", status: http.StatusOK},
		{method: http.MethodGet, path: "/api/v1/events?limit=1", status: http.StatusOK},
		{method: http.MethodPost, path: "/api/v1/summary", status: http.StatusMethodNotAllowed},
	}

	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Fatalf("%s %s status = %d, want %d; body = %s", tc.method, tc.path, rec.Code, tc.status, rec.Body.String())
		}
	}
}
