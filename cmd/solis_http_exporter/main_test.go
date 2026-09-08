package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"

	"github.com/farrokhi/solis-http-exporter/internal/config"
)

func serve(t *testing.T, opts options, target string) *httptest.ResponseRecorder {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())

	mux := newMux(reg, opts, slog.New(slog.DiscardHandler))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

func TestRoutes(t *testing.T) {
	opts := options{telemetryPath: "/metrics"}

	for _, tt := range []struct {
		path string
		want string
	}{
		{"/metrics", "go_goroutines"},
		{"/-/healthy", "OK"},
		{"/", "Solis exporter"},
	} {
		t.Run(tt.path, func(t *testing.T) {
			rec := serve(t, opts, tt.path)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			if !strings.Contains(rec.Body.String(), tt.want) {
				t.Errorf("body does not mention %q", tt.want)
			}
		})
	}
}

func TestTelemetryPathIsConfigurable(t *testing.T) {
	opts := options{telemetryPath: "/solis"}

	if rec := serve(t, opts, "/solis"); rec.Code != http.StatusOK {
		t.Errorf("/solis status = %d, want 200", rec.Code)
	}
	if rec := serve(t, opts, "/metrics"); rec.Code != http.StatusNotFound {
		t.Errorf("/metrics status = %d, want 404", rec.Code)
	}
}

func TestTargetsMapConfig(t *testing.T) {
	cfg := &config.Config{Inverters: []config.Inverter{{
		Name:     "house",
		Scheme:   "http",
		Address:  "2001:db8::1",
		Port:     8080,
		Path:     "/inverter.cgi",
		Username: "admin",
		Password: "secret",
		Timeout:  config.Duration(time.Second),
	}}}

	got := targets(cfg)
	if len(got) != 1 || got[0].Name != "house" {
		t.Fatalf("targets = %+v", got)
	}
	// A bare IPv6 address has to come back bracketed.
	if url := got[0].Fetcher.(interface{ URL() string }).URL(); url != "http://[2001:db8::1]:8080/inverter.cgi" {
		t.Errorf("URL = %s", url)
	}
}

func TestNewLoggerRejectsBadLevel(t *testing.T) {
	if _, err := newLogger("chatty"); err == nil {
		t.Error("expected an error")
	}
	if _, err := newLogger("debug"); err != nil {
		t.Errorf("newLogger: %v", err)
	}
}
