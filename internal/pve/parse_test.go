package pve

import (
	"encoding/json"
	"testing"
)

func TestFlexFloat(t *testing.T) {
	cases := []struct {
		in    string
		valid bool
		value float64
	}{
		{`5`, true, 5},
		{`3.5`, true, 3.5},
		{`"7"`, true, 7},
		{`"7.25"`, true, 7.25},
		{`null`, false, 0},
		{`""`, false, 0},
		{`"N/A"`, false, 0},
		{`"n/a"`, false, 0},
		{`"notanumber"`, false, 0},
	}
	for _, c := range cases {
		var f FlexFloat
		if err := json.Unmarshal([]byte(c.in), &f); err != nil {
			t.Fatalf("unmarshal %q: %v", c.in, err)
		}
		if f.Valid != c.valid || (c.valid && f.Value != c.value) {
			t.Errorf("input %q: got {%v %v}, want {%v %v}", c.in, f.Value, f.Valid, c.value, c.valid)
		}
	}
}

func TestParseDueDate(t *testing.T) {
	if _, ok := parseDueDate(""); ok {
		t.Error("empty date should be invalid")
	}
	if _, ok := parseDueDate("not-a-date"); ok {
		t.Error("garbage date should be invalid")
	}
	ts, ok := parseDueDate("2024-04-17")
	if !ok || ts <= 0 {
		t.Errorf("valid date should parse, got %v %v", ts, ok)
	}
}

func TestFormatInt(t *testing.T) {
	if got := formatInt(100.0); got != "100" {
		t.Errorf("formatInt(100.0) = %q, want 100", got)
	}
}
