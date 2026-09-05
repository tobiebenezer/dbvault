// Package cron implements the 5-field cron subset needed by the dbvault
// scheduler: minute hour day-of-month month day-of-week, with "*", lists,
// ranges and "*/step" entries. It is dependency-free so the restricted build
// stays green.
package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type cronExpr struct {
	minute []int
	hour   []int
	dom    []int // 1-31; nil means "*"
	month  []int
	dow    []int // 0-6 (7 normalised to 0); nil means "*"
}

var cronBounds = [5][2]int{
	{0, 59}, // minute
	{0, 23}, // hour
	{1, 31}, // day of month
	{1, 12}, // month
	{0, 7},  // day of week (7 == Sunday, normalised to 0)
}

var (
	cronMonthNames = map[string]int{
		"JAN": 1, "FEB": 2, "MAR": 3, "APR": 4, "MAY": 5, "JUN": 6,
		"JUL": 7, "AUG": 8, "SEP": 9, "OCT": 10, "NOV": 11, "DEC": 12,
	}
	cronDowNames = map[string]int{
		"SUN": 0, "MON": 1, "TUE": 2, "WED": 3, "THU": 4, "FRI": 5, "SAT": 6,
	}
)

func cronFieldNames(field int) map[string]int {
	switch field {
	case 3:
		return cronMonthNames
	case 4:
		return cronDowNames
	}
	return nil
}

// cronToken resolves one numeric-or-named token against the optional name
// table for its field.
func cronToken(tok string, names map[string]int) (int, error) {
	if names != nil {
		if v, ok := names[strings.ToUpper(tok)]; ok {
			return v, nil
		}
	}
	v, err := strconv.Atoi(tok)
	if err != nil {
		return 0, fmt.Errorf("invalid value %q", tok)
	}
	return v, nil
}

// ComputeNextFire parses a 5-field cron expression and calculates the next fire time strictly after 'after' in 'loc'.
func ComputeNextFire(spec string, after time.Time, loc *time.Location) (time.Time, error) {
	e, err := parseCron(spec)
	if err != nil {
		return time.Time{}, err
	}
	return e.NextAfter(after, loc), nil
}

func parseCron(spec string) (*cronExpr, error) {
	fields := strings.Fields(strings.TrimSpace(spec))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron expression %q must have exactly 5 fields", spec)
	}
	e := &cronExpr{}
	for i, f := range fields {
		vals, star, err := parseCronField(f, cronBounds[i][0], cronBounds[i][1], cronFieldNames(i))
		if err != nil {
			return nil, fmt.Errorf("cron field %d in %q: %w", i+1, spec, err)
		}
		if i == 4 {
			for k, v := range vals {
				if v == 7 {
					vals[k] = 0
				}
			}
		}
		switch i {
		case 0:
			e.minute = vals
		case 1:
			e.hour = vals
		case 2:
			if !star {
				e.dom = vals
			}
		case 3:
			e.month = vals
		case 4:
			if !star {
				e.dow = vals
			}
		}
	}
	return e, nil
}

// parseCronField expands one comma-separated cron entry into sorted values.
// star reports whether the entry was a bare "*" (or */step), which matters
// for the standard DOM/DOW OR rule. names optionally maps symbolic tokens
// (e.g. MON, JAN) to their numeric values.
func parseCronField(field string, lo, hi int, names map[string]int) ([]int, bool, error) {
	seen := map[int]bool{}
	out := []int{}
	star := false
	for _, part := range strings.Split(field, ",") {
		step := 1
		rangePart := part
		if idx := strings.Index(part, "/"); idx >= 0 {
			rangePart = part[:idx]
			s, err := strconv.Atoi(part[idx+1:])
			if err != nil || s <= 0 {
				return nil, false, fmt.Errorf("invalid step %q", part)
			}
			step = s
		}
		start, end := lo, hi
		switch {
		case rangePart == "*" || rangePart == "":
			star = true
		case strings.Contains(rangePart, "-"):
			bits := strings.SplitN(rangePart, "-", 2)
			a, errA := cronToken(bits[0], names)
			b, errB := cronToken(bits[1], names)
			if errA != nil || errB != nil {
				return nil, false, fmt.Errorf("invalid range %q", rangePart)
			}
			start, end = a, b
		default:
			v, err := cronToken(rangePart, names)
			if err != nil {
				return nil, false, fmt.Errorf("invalid value %q", rangePart)
			}
			start, end = v, v
		}
		if start < lo || end > hi || start > end {
			return nil, false, fmt.Errorf("value %q out of range [%d,%d]", rangePart, lo, hi)
		}
		for v := start; v <= end; v += step {
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
			if step == 1 {
				continue
			}
		}
	}
	if len(out) == 0 {
		return nil, false, fmt.Errorf("empty field %q", field)
	}
	sortInts(out)
	return out, star, nil
}

func sortInts(v []int) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}

func containsInt(vals []int, v int) bool {
	for _, x := range vals {
		if x == v {
			return true
		}
	}
	return false
}

// NextAfter returns the next fire time strictly after t, evaluated in loc.
// It scans forward day-by-day and caps the search at five years.
func (e *cronExpr) NextAfter(after time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	start := after.In(loc).Truncate(time.Minute).Add(time.Minute)
	day := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	limit := day.AddDate(5, 0, 0)
	for day.Before(limit) {
		if e.dayMatches(day) {
			for _, h := range e.hour {
				for _, m := range e.minute {
					candidate := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc)
					if !candidate.Before(start) {
						return candidate
					}
				}
			}
		}
		day = day.AddDate(0, 0, 1)
	}
	return time.Time{}
}

func (e *cronExpr) dayMatches(day time.Time) bool {
	if !containsInt(e.month, int(day.Month())) {
		return false
	}
	domMatch := e.dom != nil && containsInt(e.dom, day.Day())
	dowMatch := e.dow != nil && containsInt(e.dow, int(day.Weekday()))
	if e.dom == nil && e.dow == nil {
		return true
	}
	if e.dom == nil {
		return dowMatch
	}
	if e.dow == nil {
		return domMatch
	}
	return domMatch || dowMatch
}
