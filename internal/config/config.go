// Package config loads and validates the exporter's YAML configuration.
package config

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Secret is a password. Its String and LogValue methods keep it out of
// anything formatted or logged.
type Secret string

func (Secret) String() string       { return "[REDACTED]" }
func (Secret) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }

// Duration is a time.Duration written the usual way, as "5s".
type Duration time.Duration

func (d Duration) String() string { return time.Duration(d).String() }

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	var s string
	if err := node.Decode(&s); err != nil {
		return err
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// Inverter is one configured logger endpoint.
type Inverter struct {
	Name         string   `yaml:"name"`
	Address      string   `yaml:"address"`
	Port         int      `yaml:"port"`
	Username     string   `yaml:"username"`
	Password     Secret   `yaml:"password"`
	PasswordFile string   `yaml:"password_file"`
	Timeout      Duration `yaml:"timeout"`
	Scheme       string   `yaml:"scheme"`
	Path         string   `yaml:"path"`
}

// Config is the whole file.
type Config struct {
	Inverters []Inverter `yaml:"inverters"`
}

// Load reads, defaults and validates a configuration file. Passwords held in
// password_file are read in and folded into Password.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	for i := range cfg.Inverters {
		cfg.Inverters[i].applyDefaults()
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("%s:\n%w", path, err)
	}
	return &cfg, nil
}

func (inv *Inverter) applyDefaults() {
	inv.Username = cmp.Or(inv.Username, "admin")
	inv.Scheme = cmp.Or(inv.Scheme, "http")
	inv.Path = cmp.Or(inv.Path, "/inverter.cgi")
	inv.Timeout = cmp.Or(inv.Timeout, Duration(5*time.Second))

	if inv.Port == 0 {
		inv.Port = 80
		if inv.Scheme == "https" {
			inv.Port = 443
		}
	}
}

func (c *Config) validate() error {
	if len(c.Inverters) == 0 {
		return errors.New("no inverters configured")
	}

	var errs []error
	seen := make(map[string]bool, len(c.Inverters))

	for i := range c.Inverters {
		inv := &c.Inverters[i]

		id := inv.Name
		switch {
		case id == "":
			id = "inverters[" + strconv.Itoa(i) + "]"
			errs = append(errs, fmt.Errorf("%s: name is required", id))
		case seen[id]:
			errs = append(errs, fmt.Errorf("%s: duplicate name", id))
		default:
			seen[id] = true
		}

		if inv.Address == "" {
			errs = append(errs, fmt.Errorf("%s: address is required", id))
		}
		if inv.Port < 1 || inv.Port > 65535 {
			errs = append(errs, fmt.Errorf("%s: port %d is out of range", id, inv.Port))
		}
		if inv.Timeout <= 0 {
			errs = append(errs, fmt.Errorf("%s: timeout must be positive", id))
		}
		if inv.Scheme != "http" && inv.Scheme != "https" {
			errs = append(errs, fmt.Errorf("%s: scheme %q must be http or https", id, inv.Scheme))
		}
		if !strings.HasPrefix(inv.Path, "/") {
			errs = append(errs, fmt.Errorf("%s: path %q must start with /", id, inv.Path))
		}

		switch {
		case inv.Password != "" && inv.PasswordFile != "":
			errs = append(errs, fmt.Errorf("%s: set password or password_file, not both", id))
		case inv.Password == "" && inv.PasswordFile == "":
			errs = append(errs, fmt.Errorf("%s: password or password_file is required", id))
		case inv.PasswordFile != "":
			pw, err := readPassword(inv.PasswordFile)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", id, err))
			}
			inv.Password = pw
		}
	}

	return errors.Join(errs...)
}

func readPassword(path string) (Secret, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	pw := strings.TrimRight(string(data), "\r\n")
	if pw == "" {
		return "", fmt.Errorf("password file %s is empty", path)
	}
	return Secret(pw), nil
}
