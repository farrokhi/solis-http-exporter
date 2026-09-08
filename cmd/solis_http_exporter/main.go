// Command solis_http_exporter serves Prometheus metrics read from the local
// HTTP interface of one or more Solis inverter Wi-Fi loggers.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"

	"github.com/farrokhi/solis-http-exporter/internal/config"
	"github.com/farrokhi/solis-http-exporter/internal/exporter"
	"github.com/farrokhi/solis-http-exporter/internal/inverter"
)

// Overwritten at release time with -ldflags.
var (
	version = "1.0.0"
	commit  = ""
)

type options struct {
	configFile    string
	listenAddress string
	telemetryPath string
	logLevel      string
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	var opts options

	cmd := &cobra.Command{
		Use:   "solis_http_exporter",
		Short: "Prometheus exporter for Solis inverter Wi-Fi loggers",
		Long: "Reads Solis inverter Wi-Fi loggers over the local network and serves what\n" +
			"they report as Prometheus metrics. Nothing leaves your LAN.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		Version:      versionString(),
		RunE:         func(*cobra.Command, []string) error { return run(opts) },
	}

	f := cmd.Flags()
	f.StringVar(&opts.configFile, "config.file", "/etc/solis-http-exporter/config.yml", "configuration file to read")
	f.StringVar(&opts.listenAddress, "web.listen-address", ":9613",
		"address to serve on; give an IP to bind one interface, as in 127.0.0.1:9613")
	f.StringVar(&opts.telemetryPath, "web.telemetry-path", "/metrics", "path to serve metrics under")
	f.StringVar(&opts.logLevel, "log.level", "info", "one of debug, info, warn or error")

	cmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print the version and exit",
		Args:  cobra.NoArgs,
		Run:   func(c *cobra.Command, _ []string) { fmt.Fprintln(c.OutOrStdout(), versionString()) },
	})

	return cmd
}

func run(opts options) error {
	logger, err := newLogger(opts.logLevel)
	if err != nil {
		return err
	}

	cfg, err := config.Load(opts.configFile)
	if err != nil {
		return err
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		exporter.New(targets(cfg), logger),
	)

	srv := &http.Server{
		Addr:              opts.listenAddress,
		Handler:           newMux(reg, opts, logger),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	logger.Info("starting exporter",
		"listen", opts.listenAddress, "targets", len(cfg.Inverters), "version", versionString())

	serving := make(chan error, 1)
	go func() { serving <- srv.ListenAndServe() }()

	select {
	case err := <-serving:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdown)
	}
}

func targets(cfg *config.Config) []exporter.Target {
	out := make([]exporter.Target, len(cfg.Inverters))
	for i, inv := range cfg.Inverters {
		out[i] = exporter.Target{
			Name: inv.Name,
			Fetcher: inverter.New(inverter.Options{
				Scheme:   inv.Scheme,
				Address:  inv.Address,
				Port:     inv.Port,
				Path:     inv.Path,
				Username: inv.Username,
				Password: string(inv.Password),
				Timeout:  time.Duration(inv.Timeout),
			}),
		}
	}
	return out
}

func newMux(reg *prometheus.Registry, opts options, logger *slog.Logger) *http.ServeMux {
	mux := http.NewServeMux()

	mux.Handle("GET "+opts.telemetryPath, promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorLog:      slog.NewLogLogger(logger.Handler(), slog.LevelError),
		ErrorHandling: promhttp.ContinueOnError,
	}))

	// Reports on the exporter itself, not on whether the inverters answer.
	mux.HandleFunc("GET /-/healthy", func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, "OK\n")
	})

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, landingPage, opts.telemetryPath, opts.telemetryPath)
	})

	return mux
}

const landingPage = `<!doctype html>
<title>Solis exporter</title>
<h1>Solis exporter</h1>
<p><a href="%s">%s</a></p>
`

func newLogger(level string) (*slog.Logger, error) {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("invalid log level %q", level)
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: l})), nil
}

func versionString() string {
	if commit == "" {
		return version
	}
	return version + " (" + commit + ")"
}
