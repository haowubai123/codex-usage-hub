package server

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
)

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/ingest", s.IngestHandler)
	mux.HandleFunc("/api/v1/summary", s.SummaryHandler)
	mux.HandleFunc("/api/v1/breakdown", s.BreakdownHandler)
	mux.HandleFunc("/api/v1/events", s.EventsHandler)
	mux.HandleFunc("/healthz", s.HealthHandler)
	mux.HandleFunc("/", s.dashboardHandler)
	return mux
}

func (s Server) HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true}` + "\n"))
}

func (s Server) dashboardHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/" && r.URL.Path != "/index.html" {
		http.NotFound(w, r)
		return
	}

	data, err := readDashboardHTML()
	if err != nil {
		http.Error(w, "dashboard not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func readDashboardHTML() ([]byte, error) {
	if _, file, _, ok := runtime.Caller(0); ok {
		path := filepath.Join(filepath.Dir(file), "..", "..", "web", "index.html")
		if data, err := os.ReadFile(path); err == nil {
			return data, nil
		}
	}
	return os.ReadFile(filepath.Join("web", "index.html"))
}
