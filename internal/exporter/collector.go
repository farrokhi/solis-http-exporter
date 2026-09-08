// Package exporter turns inverter readings into Prometheus metrics.
package exporter

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/farrokhi/solis-http-exporter/internal/inverter"
)

const joulesPerKWh = 3.6e6

// Fetcher reads one status.
type Fetcher interface {
	Fetch(ctx context.Context) (inverter.Status, error)
}

// Target is a configured inverter under its reporting name.
type Target struct {
	Name    string
	Fetcher Fetcher
}

var (
	targetLabel = []string{"target"}

	upDesc = prometheus.NewDesc("solis_up",
		"Whether the inverter was successfully scraped.", targetLabel, nil)
	durationDesc = prometheus.NewDesc("solis_scrape_duration_seconds",
		"Time taken to scrape one inverter.", targetLabel, nil)
	powerDesc = prometheus.NewDesc("solis_inverter_power_watts",
		"Current inverter output power.", targetLabel, nil)
	temperatureDesc = prometheus.NewDesc("solis_inverter_temperature_celsius",
		"Current inverter temperature.", targetLabel, nil)
	todayDesc = prometheus.NewDesc("solis_inverter_energy_today_joules",
		"Energy produced today.", targetLabel, nil)
	totalDesc = prometheus.NewDesc("solis_inverter_energy_joules_total",
		"Energy produced over the inverter's lifetime.", targetLabel, nil)
	alertDesc = prometheus.NewDesc("solis_inverter_alert_active",
		"Whether the inverter is reporting an alert.", targetLabel, nil)
	infoDesc = prometheus.NewDesc("solis_inverter_info",
		"Constant metric labelled with the inverter's identity.",
		[]string{"target", "serial", "firmware", "model"}, nil)
)

// Collector scrapes every configured inverter on demand.
type Collector struct {
	targets []Target
	log     *slog.Logger
}

func New(targets []Target, log *slog.Logger) *Collector {
	return &Collector{targets: targets, log: log}
}

// Describe sends nothing, making this an unchecked collector: which metrics
// exist depends on what each inverter answered.
func (c *Collector) Describe(chan<- *prometheus.Desc) {}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	var wg sync.WaitGroup
	for _, t := range c.targets {
		wg.Go(func() { c.scrape(context.Background(), ch, t) })
	}
	wg.Wait()
}

func (c *Collector) scrape(ctx context.Context, ch chan<- prometheus.Metric, t Target) {
	start := time.Now()
	status, err := t.Fetcher.Fetch(ctx)
	gauge(ch, durationDesc, time.Since(start).Seconds(), t.Name)

	if err != nil {
		c.log.Warn("unable to read inverter", "target", t.Name, "error", err)
		gauge(ch, upDesc, 0, t.Name)
		return
	}
	gauge(ch, upDesc, 1, t.Name)

	for _, field := range status.BadFields {
		c.log.Warn("invalid inverter field", "target", t.Name, "field", field.Name, "value", field.Raw)
	}
	if status.AlertActive() {
		c.log.Warn("inverter reports an alert", "target", t.Name, "alert", status.Alert)
	}
	c.log.Debug("inverter scraped", "target", t.Name, "serial", status.Serial)

	if v := status.PowerWatts; v != nil {
		gauge(ch, powerDesc, *v, t.Name)
	}
	if v := status.TemperatureCelsius; v != nil {
		gauge(ch, temperatureDesc, *v, t.Name)
	}
	if v := status.EnergyTodayKWh; v != nil {
		gauge(ch, todayDesc, *v*joulesPerKWh, t.Name)
	}
	if v := status.EnergyTotalKWh; v != nil {
		ch <- prometheus.MustNewConstMetric(totalDesc, prometheus.CounterValue, *v*joulesPerKWh, t.Name)
	}
	if status.Alert != "" {
		gauge(ch, alertDesc, boolean(status.AlertActive()), t.Name)
	}

	ch <- prometheus.MustNewConstMetric(infoDesc, prometheus.GaugeValue, 1,
		t.Name, status.Serial, status.Firmware, status.Model)
}

func gauge(ch chan<- prometheus.Metric, d *prometheus.Desc, v float64, labels ...string) {
	ch <- prometheus.MustNewConstMetric(d, prometheus.GaugeValue, v, labels...)
}

func boolean(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
