package inverter

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestParseValid(t *testing.T) {
	tests := []struct {
		file      string
		alert     string
		active    bool
		badFields []string
		temp      *float64
		power     *float64
		today     *float64
		total     *float64
	}{
		{
			file:  "inverter-normal.txt",
			alert: "NO",
			temp:  new(40.1), power: new(900.0), today: new(5.4), total: new(34293.5),
		},
		{
			file:  "inverter-crlf.txt",
			alert: "NO",
			temp:  new(40.1), power: new(900.0), today: new(5.4), total: new(34293.5),
		},
		{
			file:  "inverter-whitespace.txt",
			alert: "NO",
			temp:  new(40.1), power: new(900.0), today: new(5.4), total: new(34293.5),
		},
		{
			file:  "inverter-control-chars.txt",
			alert: "NO",
			temp:  new(40.1), power: new(900.0), today: new(5.4), total: new(34293.5),
		},
		{
			file:  "inverter-missing-total.txt",
			alert: "NO", badFields: []string{"yield_total"},
			temp: new(40.1), power: new(900.0), today: new(5.4), total: nil,
		},
		{
			file:  "inverter-bad-power.txt",
			alert: "NO", badFields: []string{"current_power"},
			temp: new(40.1), power: nil, today: new(5.4), total: new(34293.5),
		},
		{
			file:  "inverter-bad-temperature.txt",
			alert: "NO", badFields: []string{"temperature"},
			temp: nil, power: new(900.0), today: new(5.4), total: new(34293.5),
		},
		{
			file:  "inverter-alert.txt",
			alert: "F23", active: true,
			temp: new(40.1), power: new(900.0), today: new(5.4), total: new(34293.5),
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got, err := Parse(load(t, tt.file))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if got.Serial != "1802020228090133" || got.Firmware != "780036" || got.Model != "202" {
				t.Errorf("identity = %q/%q/%q", got.Serial, got.Firmware, got.Model)
			}
			if got.Alert != tt.alert {
				t.Errorf("Alert = %q, want %q", got.Alert, tt.alert)
			}
			if got.AlertActive() != tt.active {
				t.Errorf("AlertActive() = %v, want %v", got.AlertActive(), tt.active)
			}
			checkFloat(t, "TemperatureCelsius", got.TemperatureCelsius, tt.temp)
			checkFloat(t, "PowerWatts", got.PowerWatts, tt.power)
			checkFloat(t, "EnergyTodayKWh", got.EnergyTodayKWh, tt.today)
			checkFloat(t, "EnergyTotalKWh", got.EnergyTotalKWh, tt.total)
			checkBadFields(t, got.BadFields, tt.badFields)
		})
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		file string
		want error
	}{
		{"inverter-empty.txt", ErrEmptyResponse},
		{"inverter-short.txt", ErrShortRecord},
		{"inverter-malformed.txt", ErrShortRecord},
		{"inverter-zero-serial.txt", ErrInvalidSerial},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			if _, err := Parse(load(t, tt.file)); !errors.Is(err, tt.want) {
				t.Errorf("Parse error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestParseRejectsNonFinite(t *testing.T) {
	got, err := Parse([]byte("1802020228090133;780036;202;NaN;+Inf;5.4;34293.5;NO;"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.TemperatureCelsius != nil || got.PowerWatts != nil {
		t.Error("NaN and Inf should not produce values")
	}
	checkBadFields(t, got.BadFields, []string{"temperature", "current_power"})
}

func TestParseRejectsNegativeEnergyAndPower(t *testing.T) {
	got, err := Parse([]byte("1802020228090133;780036;202;-4.5;-1;-0.2;-34293.5;NO;"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.PowerWatts != nil || got.EnergyTodayKWh != nil || got.EnergyTotalKWh != nil {
		t.Error("negative power and energy should not produce values")
	}
	if got.TemperatureCelsius == nil || *got.TemperatureCelsius != -4.5 {
		t.Error("a freezing inverter is a real reading")
	}
	checkBadFields(t, got.BadFields, []string{"current_power", "yield_today", "yield_total"})
}

func TestParseEmptyAlert(t *testing.T) {
	got, err := Parse([]byte("1802020228090133;780036;202;40.1;900;5.4;34293.5;;"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checkBadFields(t, got.BadFields, []string{"alerts"})
}

func checkFloat(t *testing.T, name string, got, want *float64) {
	t.Helper()
	switch {
	case got == nil && want == nil:
	case got == nil || want == nil:
		t.Errorf("%s = %v, want %v", name, got, want)
	case *got != *want:
		t.Errorf("%s = %v, want %v", name, *got, *want)
	}
}

func checkBadFields(t *testing.T, got []BadField, want []string) {
	t.Helper()
	names := make([]string, len(got))
	for i, f := range got {
		names[i] = f.Name
	}
	if !slices.Equal(names, want) {
		t.Fatalf("BadFields = %v, want %v", names, want)
	}
}

// Captured from an E3 logger on firmware 49002F, which puts the letter "d"
// where the spec expects a lifetime yield. The serial is anonymised.
func TestParseE3FirmwareWithoutLifetimeYield(t *testing.T) {
	got, err := Parse(load(t, "inverter-e3-no-total.txt"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Model != "E3" || got.Firmware != "49002F" {
		t.Errorf("identity = %q/%q", got.Firmware, got.Model)
	}
	checkFloat(t, "TemperatureCelsius", got.TemperatureCelsius, new(32.1))
	checkFloat(t, "PowerWatts", got.PowerWatts, new(100.0))
	checkFloat(t, "EnergyTodayKWh", got.EnergyTodayKWh, new(0.4))
	checkFloat(t, "EnergyTotalKWh", got.EnergyTotalKWh, nil)
	if got.AlertActive() {
		t.Error("NO is not an alert")
	}
	if len(got.BadFields) != 1 || got.BadFields[0].OutOfRange {
		t.Errorf("BadFields = %+v, want yield_total unreported rather than out of range", got.BadFields)
	}
}

func TestOutOfRangeIsDistinctFromUnreported(t *testing.T) {
	got, err := Parse([]byte("1802020228090133;780036;202;40.1;-1;5.4;;NO;"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	checkBadFields(t, got.BadFields, []string{"current_power", "yield_total"})
	if !got.BadFields[0].OutOfRange {
		t.Error("a negative power is a number that cannot be real")
	}
	if got.BadFields[1].OutOfRange {
		t.Error("an empty field was never a number")
	}
}

func TestBadFieldCarriesTheRawValue(t *testing.T) {
	got, err := Parse([]byte("1802020228090133;780036;202;40.1;900;5.4;n/a;NO;"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(got.BadFields) != 1 || got.BadFields[0] != (BadField{Name: "yield_total", Raw: "n/a"}) {
		t.Errorf("BadFields = %+v, want the offending value kept", got.BadFields)
	}
}
