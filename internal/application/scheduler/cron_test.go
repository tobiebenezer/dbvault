package scheduler

import (
	"testing"
	"time"
)

func TestParseCronValid(t *testing.T) {
	e, err := parseCron("*/15 9-17 * * MON-FRI")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.minute) != 4 || e.minute[0] != 0 || e.minute[3] != 45 {
		t.Fatalf("minute expansion wrong: %v", e.minute)
	}
	if len(e.hour) != 9 || e.hour[0] != 9 || e.hour[8] != 17 {
		t.Fatalf("hour range wrong: %v", e.hour)
	}
	if e.dom != nil {
		t.Fatalf("bare dom should stay nil: %v", e.dom)
	}
	if !containsInt(e.dow, 1) || !containsInt(e.dow, 5) || containsInt(e.dow, 6) {
		t.Fatalf("dow range wrong: %v", e.dow)
	}
}

func TestParseCronSundayNormalised(t *testing.T) {
	e, err := parseCron("0 0 * * 7")
	if err != nil {
		t.Fatal(err)
	}
	if len(e.dow) != 1 || e.dow[0] != 0 {
		t.Fatalf("dow 7 must normalise to 0: %v", e.dow)
	}
}

func TestParseCronInvalid(t *testing.T) {
	cases := []string{"", "* * * *", "61 * * * *", "* 25 * * *", "*/0 * * * *", "a * * * *", "5-1 * * * *"}
	for _, c := range cases {
		if _, err := parseCron(c); err == nil {
			t.Fatalf("expected error for %q", c)
		}
	}
}

func TestNextAfterBasic(t *testing.T) {
	e, err := parseCron("30 2 * * *")
	if err != nil {
		t.Fatal(err)
	}
	loc := time.UTC
	after := time.Date(2026, 8, 22, 10, 0, 0, 0, loc)
	next := e.NextAfter(after, loc)
	want := time.Date(2026, 8, 23, 2, 30, 0, 0, loc)
	if !next.Equal(want) {
		t.Fatalf("next=%v want=%v", next, want)
	}
}

func TestNextAfterSameMinuteExclusive(t *testing.T) {
	e, _ := parseCron("*/15 * * * *")
	at := time.Date(2026, 8, 22, 9, 15, 0, 0, time.UTC)
	next := e.NextAfter(at, time.UTC)
	want := time.Date(2026, 8, 22, 9, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next=%v want strictly after fire instant: %v", next, want)
	}
}

func TestNextAfterTimezone(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	e, _ := parseCron("0 9 * * *")
	after := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) // 14:00 Berlin
	next := e.NextAfter(after, loc)
	want := time.Date(2026, 8, 23, 9, 0, 0, 0, loc) // next day 09:00 local
	if !next.Equal(want) {
		t.Fatalf("next=%v want=%v", next, want)
	}
}

func TestNextAfterDomDowOrRule(t *testing.T) {
	// Aug 22 2026 is a Saturday (dow=6). dom=1 and dow=1(Mon): OR rule.
	e, err := parseCron("0 0 1 * 1")
	if err != nil {
		t.Fatal(err)
	}
	after := time.Date(2026, 8, 22, 1, 0, 0, 0, time.UTC)
	next := e.NextAfter(after, time.UTC)
	// Mon Aug 24 (dow match) comes before Sat Sep 1... wait Sep 1 is later; Monday wins.
	want := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next=%v want=%v", next, want)
	}
}
