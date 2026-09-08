// Package inverter reads and decodes the status record served by a Solis
// Wi-Fi logging stick at /inverter.cgi.
package inverter

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Positions in the semicolon-separated record.
const (
	fieldSerial = iota
	fieldFirmware
	fieldModel
	fieldTemperature
	fieldPower
	fieldYieldToday
	fieldYieldTotal
	fieldAlerts
	fieldCount
)

// Structural failures. Each one means the whole reading is unusable.
var (
	ErrEmptyResponse = errors.New("empty response")
	ErrShortRecord   = errors.New("short record")
	ErrInvalidSerial = errors.New("invalid serial")
)

// Status is one reading from an inverter. A nil pointer means the logger did
// not supply a usable value, which is not the same as zero.
type Status struct {
	Serial   string
	Firmware string
	Model    string

	TemperatureCelsius *float64
	PowerWatts         *float64
	EnergyTodayKWh     *float64
	EnergyTotalKWh     *float64

	Alert string

	// BadFields holds the fields that failed to parse, for logging.
	BadFields []BadField
}

// BadField is a field the logger sent that could not be used.
type BadField struct {
	Name string
	Raw  string
	// OutOfRange separates a number that cannot be real from a field the
	// logger simply does not report, which some firmware never will.
	OutOfRange bool
}

// AlertActive reports whether the logger is flagging anything.
func (s Status) AlertActive() bool {
	return s.Alert != "" && !strings.EqualFold(s.Alert, "NO")
}

// Parse decodes a record. A non-nil error means the response was structurally
// unusable; unparseable individual fields are reported in Status.BadFields.
func Parse(data []byte) (Status, error) {
	fields := split(string(data))
	if len(fields) == 0 {
		return Status{}, ErrEmptyResponse
	}
	if len(fields) < fieldCount {
		return Status{}, fmt.Errorf("%w: %d fields, want %d", ErrShortRecord, len(fields), fieldCount)
	}

	s := Status{
		Serial:   fields[fieldSerial],
		Firmware: fields[fieldFirmware],
		Model:    fields[fieldModel],
		Alert:    fields[fieldAlerts],
	}

	// An all-zero serial means the inverter is still booting and the rest is junk.
	if strings.Trim(s.Serial, "0") == "" {
		return Status{}, ErrInvalidSerial
	}

	number := func(raw, name string, lowest float64) *float64 {
		v, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		switch {
		case err != nil || math.IsNaN(v) || math.IsInf(v, 0):
			s.BadFields = append(s.BadFields, BadField{Name: name, Raw: raw})
		case v < lowest:
			s.BadFields = append(s.BadFields, BadField{Name: name, Raw: raw, OutOfRange: true})
		default:
			return &v
		}
		return nil
	}

	// Only temperature can sensibly be below zero.
	s.TemperatureCelsius = number(fields[fieldTemperature], "temperature", math.Inf(-1))
	s.PowerWatts = number(fields[fieldPower], "current_power", 0)
	s.EnergyTodayKWh = number(fields[fieldYieldToday], "yield_today", 0)
	s.EnergyTotalKWh = number(fields[fieldYieldTotal], "yield_total", 0)

	if s.Alert == "" {
		s.BadFields = append(s.BadFields, BadField{Name: "alerts"})
	}

	return s, nil
}

func split(s string) []string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)

	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	fields := strings.Split(s, ";")
	if strings.TrimSpace(fields[len(fields)-1]) == "" {
		fields = fields[:len(fields)-1]
	}
	return fields
}
