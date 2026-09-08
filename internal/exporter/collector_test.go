package exporter

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/farrokhi/solis-http-exporter/internal/inverter"
)

type fakeFetcher struct {
	status inverter.Status
	err    error
}

func (f fakeFetcher) Fetch(context.Context) (inverter.Status, error) { return f.status, f.err }

func healthy() inverter.Status {
	return inverter.Status{
		Serial:             "1802020228090133",
		Firmware:           "780036",
		Model:              "202",
		TemperatureCelsius: new(40.1),
		PowerWatts:         new(900.0),
		EnergyTodayKWh:     new(5.4),
		EnergyTotalKWh:     new(34293.5),
		Alert:              "NO",
	}
}

func collect(t *testing.T, targets []Target, want string, names ...string) {
	t.Helper()
	c := New(targets, slog.New(slog.DiscardHandler))
	if err := testutil.CollectAndCompare(c, strings.NewReader(want), names...); err != nil {
		t.Error(err)
	}
}

func TestCollectOneInverter(t *testing.T) {
	targets := []Target{{Name: "house", Fetcher: fakeFetcher{status: healthy()}}}

	collect(t, targets, `
# HELP solis_up Whether the inverter was successfully scraped.
# TYPE solis_up gauge
solis_up{target="house"} 1
# HELP solis_inverter_power_watts Current inverter output power.
# TYPE solis_inverter_power_watts gauge
solis_inverter_power_watts{target="house"} 900
# HELP solis_inverter_temperature_celsius Current inverter temperature.
# TYPE solis_inverter_temperature_celsius gauge
solis_inverter_temperature_celsius{target="house"} 40.1
# HELP solis_inverter_energy_today_joules Energy produced today.
# TYPE solis_inverter_energy_today_joules gauge
solis_inverter_energy_today_joules{target="house"} 1.944e+07
# HELP solis_inverter_energy_joules_total Energy produced over the inverter's lifetime.
# TYPE solis_inverter_energy_joules_total counter
solis_inverter_energy_joules_total{target="house"} 1.234566e+11
# HELP solis_inverter_alert_active Whether the inverter is reporting an alert.
# TYPE solis_inverter_alert_active gauge
solis_inverter_alert_active{target="house"} 0
# HELP solis_inverter_info Constant metric labelled with the inverter's identity.
# TYPE solis_inverter_info gauge
solis_inverter_info{firmware="780036",model="202",serial="1802020228090133",target="house"} 1
`,
		"solis_up", "solis_inverter_power_watts", "solis_inverter_temperature_celsius",
		"solis_inverter_energy_today_joules", "solis_inverter_energy_joules_total",
		"solis_inverter_alert_active", "solis_inverter_info")
}

func TestCollectMultipleInverters(t *testing.T) {
	garage := healthy()
	garage.Serial = "1802020228090134"
	garage.PowerWatts = new(1250.0)

	targets := []Target{
		{Name: "house", Fetcher: fakeFetcher{status: healthy()}},
		{Name: "garage", Fetcher: fakeFetcher{status: garage}},
	}

	collect(t, targets, `
# HELP solis_up Whether the inverter was successfully scraped.
# TYPE solis_up gauge
solis_up{target="garage"} 1
solis_up{target="house"} 1
# HELP solis_inverter_power_watts Current inverter output power.
# TYPE solis_inverter_power_watts gauge
solis_inverter_power_watts{target="garage"} 1250
solis_inverter_power_watts{target="house"} 900
`, "solis_up", "solis_inverter_power_watts")
}

func TestCollectIsolatesFailure(t *testing.T) {
	targets := []Target{
		{Name: "house", Fetcher: fakeFetcher{status: healthy()}},
		{Name: "garage", Fetcher: fakeFetcher{err: errors.New("context deadline exceeded")}},
	}

	collect(t, targets, `
# HELP solis_up Whether the inverter was successfully scraped.
# TYPE solis_up gauge
solis_up{target="garage"} 0
solis_up{target="house"} 1
# HELP solis_inverter_power_watts Current inverter output power.
# TYPE solis_inverter_power_watts gauge
solis_inverter_power_watts{target="house"} 900
# HELP solis_inverter_info Constant metric labelled with the inverter's identity.
# TYPE solis_inverter_info gauge
solis_inverter_info{firmware="780036",model="202",serial="1802020228090133",target="house"} 1
`, "solis_up", "solis_inverter_power_watts", "solis_inverter_info")
}

func TestCollectOmitsUnavailableFields(t *testing.T) {
	status := healthy()
	status.EnergyTotalKWh = nil
	status.BadFields = []inverter.BadField{{Name: "yield_total"}}

	targets := []Target{{Name: "house", Fetcher: fakeFetcher{status: status}}}

	collect(t, targets, `
# HELP solis_up Whether the inverter was successfully scraped.
# TYPE solis_up gauge
solis_up{target="house"} 1
# HELP solis_inverter_energy_today_joules Energy produced today.
# TYPE solis_inverter_energy_today_joules gauge
solis_inverter_energy_today_joules{target="house"} 1.944e+07
`, "solis_up", "solis_inverter_energy_joules_total", "solis_inverter_energy_today_joules")
}

func TestCollectReportsActiveAlert(t *testing.T) {
	status := healthy()
	status.Alert = "F23"

	targets := []Target{{Name: "house", Fetcher: fakeFetcher{status: status}}}

	collect(t, targets, `
# HELP solis_inverter_alert_active Whether the inverter is reporting an alert.
# TYPE solis_inverter_alert_active gauge
solis_inverter_alert_active{target="house"} 1
`, "solis_inverter_alert_active")
}

func TestCollectAlwaysReportsDuration(t *testing.T) {
	targets := []Target{
		{Name: "house", Fetcher: fakeFetcher{status: healthy()}},
		{Name: "garage", Fetcher: fakeFetcher{err: errors.New("connection refused")}},
	}

	c := New(targets, slog.New(slog.DiscardHandler))
	if got := testutil.CollectAndCount(c, "solis_scrape_duration_seconds"); got != 2 {
		t.Errorf("scrape duration series = %d, want 2", got)
	}
}
