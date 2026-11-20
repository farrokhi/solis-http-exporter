package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"runtime/debug"
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

func TestResolveVersion(t *testing.T) {
	stamped := func(version string, settings ...debug.BuildSetting) *debug.BuildInfo {
		return &debug.BuildInfo{Main: debug.Module{Version: version}, Settings: settings}
	}
	rev := debug.BuildSetting{Key: "vcs.revision", Value: "3ba935a8c3d1e2f4a5b6c7d8e9f0a1b2c3d4e5f6"}

	tests := []struct {
		name            string
		version, commit string
		info            *debug.BuildInfo
		want            string
	}{
		{"ldflags win over build info", "1.0.1", "3ba935a", stamped("v9.9.9", rev), "1.0.1 (3ba935a)"},
		{"go install from the module proxy", "", "", stamped("v1.0.1"), "1.0.1"},
		{"build in a git checkout", "", "", stamped("v1.0.2-0.20260908130139-3ba935a8c3d1", rev),
			"1.0.2-0.20260908130139-3ba935a8c3d1 (3ba935a)"},
		{"no build info at all", "", "", nil, "(devel)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveVersion(tt.version, tt.commit, tt.info); got != tt.want {
				t.Errorf("resolveVersion() = %q, want %q", got, tt.want)
			}
		})
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
