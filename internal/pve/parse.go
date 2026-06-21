package pve

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// FlexFloat tolerantly decodes a JSON value that may be a number, a numeric
// string, null, "" or "N/A". Unparseable or sentinel values leave Valid=false
// so callers can emit an *absent* sample rather than a misleading zero.
type FlexFloat struct {
	Value float64
	Valid bool
}

// UnmarshalJSON implements json.Unmarshaler with the tolerant rules above.
func (f *FlexFloat) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	// Quoted string form.
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		s = strings.TrimSpace(s)
		if s == "" || strings.EqualFold(s, "n/a") {
			return nil
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil // tolerate unparseable strings as absent
		}
		f.Value, f.Valid = v, true
		return nil
	}
	// Bare number form.
	v, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return nil
	}
	f.Value, f.Valid = v, true
	return nil
}

// FlexString tolerantly decodes a value that may be a string or a number into a
// string (PVE occasionally returns numeric ids as numbers).
type FlexString string

// UnmarshalJSON implements json.Unmarshaler.
func (s *FlexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*s = FlexString(str)
		return nil
	}
	*s = FlexString(strings.Trim(string(b), `"`))
	return nil
}

// parseDueDate converts a PVE subscription nextduedate ("YYYY-MM-DD") to a Unix
// timestamp. It returns (0, false) when the value is empty or unparseable.
func parseDueDate(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return 0, false
	}
	return float64(t.Unix()), true
}

// boolToFloat maps a boolean to 1.0/0.0.
func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// formatInt renders a float that represents an integer as a base-10 string.
func formatInt(v float64) string {
	return strconv.FormatInt(int64(v), 10)
}
