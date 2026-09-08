package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func load(t *testing.T, content string) (*Config, error) {
	t.Helper()
	return Load(write(t, "config.yml", content))
}

func TestLoadAppliesDefaults(t *testing.T) {
	cfg, err := load(t, `
inverters:
  - name: house
    address: 192.168.2.164
    password: secret
`)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	inv := cfg.Inverters[0]
	if inv.Username != "admin" || inv.Scheme != "http" || inv.Path != "/inverter.cgi" {
		t.Errorf("defaults not applied: %+v", inv)
	}
	if inv.Port != 80 {
		t.Errorf("Port = %d, want 80", inv.Port)
	}
	if time.Duration(inv.Timeout) != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", inv.Timeout)
	}
}

func TestLoadHTTPSDefaultsToPort443(t *testing.T) {
	cfg, err := load(t, `
inverters:
  - name: house
    address: solis.lan
    scheme: https
    password: secret
`)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Inverters[0].Port != 443 {
		t.Errorf("Port = %d, want 443", cfg.Inverters[0].Port)
	}
}

func TestLoadExplicitValues(t *testing.T) {
	cfg, err := load(t, `
inverters:
  - name: garage
    address: 2001:db8::1
    port: 8080
    username: operator
    password: secret
    timeout: 250ms
    scheme: http
    path: /status.cgi
`)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	inv := cfg.Inverters[0]
	if inv.Address != "2001:db8::1" || inv.Port != 8080 || inv.Username != "operator" || inv.Path != "/status.cgi" {
		t.Errorf("unexpected inverter %+v", inv)
	}
	if time.Duration(inv.Timeout) != 250*time.Millisecond {
		t.Errorf("Timeout = %v, want 250ms", inv.Timeout)
	}
}

func TestLoadPasswordFile(t *testing.T) {
	pwPath := write(t, "house.password", "from-a-file\n")
	cfg, err := load(t, fmt.Sprintf(`
inverters:
  - name: house
    address: 192.168.2.164
    password_file: %s
`, pwPath))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Inverters[0].Password != "from-a-file" {
		t.Errorf("Password not read from file")
	}
}

func TestLoadRejects(t *testing.T) {
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "no inverters",
			yaml: "inverters: []\n",
			want: "no inverters configured",
		},
		{
			name: "duplicate names",
			yaml: `
inverters:
  - {name: house, address: a, password: p}
  - {name: house, address: b, password: p}
`,
			want: "duplicate name",
		},
		{
			name: "missing name",
			yaml: "inverters:\n  - {address: a, password: p}\n",
			want: "name is required",
		},
		{
			name: "missing address",
			yaml: "inverters:\n  - {name: house, password: p}\n",
			want: "address is required",
		},
		{
			name: "missing credentials",
			yaml: "inverters:\n  - {name: house, address: a}\n",
			want: "password or password_file is required",
		},
		{
			name: "both credentials",
			yaml: "inverters:\n  - {name: house, address: a, password: p, password_file: /dev/null}\n",
			want: "not both",
		},
		{
			name: "unparseable timeout",
			yaml: "inverters:\n  - {name: house, address: a, password: p, timeout: 5x}\n",
			want: "unknown unit",
		},
		{
			name: "negative timeout",
			yaml: "inverters:\n  - {name: house, address: a, password: p, timeout: -1s}\n",
			want: "timeout must be positive",
		},
		{
			name: "port out of range",
			yaml: "inverters:\n  - {name: house, address: a, password: p, port: 70000}\n",
			want: "out of range",
		},
		{
			name: "bad scheme",
			yaml: "inverters:\n  - {name: house, address: a, password: p, scheme: ftp}\n",
			want: "must be http or https",
		},
		{
			name: "relative path",
			yaml: "inverters:\n  - {name: house, address: a, password: p, path: inverter.cgi}\n",
			want: "must start with /",
		},
		{
			name: "unknown field",
			yaml: "inverters:\n  - {name: house, address: a, password: p, retries: 3}\n",
			want: "field retries not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load(t, tt.yaml)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestLoadRejectsEmptyPasswordFile(t *testing.T) {
	pwPath := write(t, "empty.password", "\n")
	_, err := load(t, fmt.Sprintf("inverters:\n  - {name: house, address: a, password_file: %s}\n", pwPath))
	if err == nil || !strings.Contains(err.Error(), "is empty") {
		t.Errorf("error = %v, want it to mention an empty password file", err)
	}
}

func TestLoadReportsEveryProblemAtOnce(t *testing.T) {
	_, err := load(t, "inverters:\n  - {name: house}\n")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"address is required", "password or password_file is required"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

func TestSecretStaysHidden(t *testing.T) {
	s := Secret("hunter2")

	if got := fmt.Sprintf("%v %s", s, s); strings.Contains(got, "hunter2") {
		t.Errorf("formatted secret leaked: %s", got)
	}
	if got := slog.AnyValue(s).String(); strings.Contains(got, "hunter2") {
		t.Errorf("logged secret leaked: %s", got)
	}
	if string(s) != "hunter2" {
		t.Error("the underlying value should still be usable")
	}
}
