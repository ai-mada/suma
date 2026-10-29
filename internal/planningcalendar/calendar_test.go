package planningcalendar

import (
	"math"
	"strings"
	"testing"
	"time"
)

func hours(value float64) *float64 { return &value }

func workweek() ResourceCalendarSchedule {
	return ResourceCalendarSchedule{Weekdays: []*float64{hours(8), hours(8), hours(8), hours(8), hours(8), hours(0), hours(0)}}
}

// Removing exception precedence, treating unknown as zero, or allocating on a
// holiday would each break the independently checked week and interval totals.
func TestCalendarExceptionsOverrideBaseWithoutConflatingUnknownAndZero(t *testing.T) {
	schedule := workweek()
	schedule.Exceptions = []ResourceCalendarException{
		{Date: "2026-10-26", AvailableHours: hours(0), Note: "Holiday"},
		{Date: "2026-10-27", AvailableHours: hours(4), Note: "Half-day leave"},
		{Date: "2026-10-28", AvailableHours: nil},
		{Date: "2026-10-31", AvailableHours: hours(2)},
	}
	days, err := Week(schedule, "2026-10-26")
	if err != nil {
		t.Fatal(err)
	}
	wantDates := []string{"2026-10-26", "2026-10-27", "2026-10-28", "2026-10-29", "2026-10-30", "2026-10-31", "2026-11-01"}
	wantHours := []*float64{hours(0), hours(4), nil, hours(8), hours(8), hours(2), hours(0)}
	wantSources := []string{"exception", "exception", "exception", "base", "base", "exception", "base"}
	if len(days) != 7 {
		t.Fatalf("got %d days; want 7", len(days))
	}
	for i, day := range days {
		if day.Date != wantDates[i] || day.Source != wantSources[i] {
			t.Errorf("day %d: got %+v; want date %s, source %s", i, day, wantDates[i], wantSources[i])
		}
		if (day.AvailableHours == nil) != (wantHours[i] == nil) || day.AvailableHours != nil && *day.AvailableHours != *wantHours[i] {
			t.Errorf("day %d: availability mismatch", i)
		}
	}
	known, unknown, err := Weight(schedule, "2026-10-26", "2026-11-01")
	if err != nil || known != 22 || unknown != 1 {
		t.Fatalf("Weight = (%v, %v, %v); want (22, 1, nil)", known, unknown, err)
	}
	known, unknown, err = Weight(ResourceCalendarSchedule{}, "2026-10-26", "2026-11-01")
	if err != nil || known != 0 || unknown != 7 {
		t.Fatalf("unset calendar = (%v, %v, %v); want (0, 7, nil)", known, unknown, err)
	}
	unsetDays, err := Week(ResourceCalendarSchedule{}, "2026-10-26")
	if err != nil {
		t.Fatal(err)
	}
	for _, day := range unsetDays {
		if day.AvailableHours != nil || day.Source != "unknown" {
			t.Fatalf("unset day must remain unknown: %+v", day)
		}
	}
}

// A natural-day denominator would assign 7/12 to the first week. Calendar
// weights split 136 as 56/80 after a holiday and four-hour leave in that week.
func TestCalendarWeightsSplitCrossWeekEstimateByAvailableHours(t *testing.T) {
	schedule := workweek()
	schedule.Exceptions = []ResourceCalendarException{
		{Date: "2026-10-30", AvailableHours: hours(0)},
		{Date: "2026-10-29", AvailableHours: hours(4)},
	}
	total, unknown, err := Weight(schedule, "2026-10-26", "2026-11-06")
	if err != nil || total != 68 || unknown != 0 {
		t.Fatalf("total = (%v, %v, %v); want (68, 0, nil)", total, unknown, err)
	}
	first, unknown, err := Weight(schedule, "2026-10-26", "2026-11-01")
	if err != nil || first != 28 || unknown != 0 {
		t.Fatalf("first = (%v, %v, %v); want (28, 0, nil)", first, unknown, err)
	}
	second, unknown, err := Weight(schedule, "2026-11-02", "2026-11-06")
	if err != nil || second != 40 || unknown != 0 {
		t.Fatalf("second = (%v, %v, %v); want (40, 0, nil)", second, unknown, err)
	}
	if 136*first/total != 56 || 136*second/total != 80 {
		t.Fatal("weighted shares must preserve the 136-hour estimate as 56/80")
	}
	// An exception can resolve an otherwise unknown day; explicit zero remains known.
	partial := ResourceCalendarSchedule{Exceptions: []ResourceCalendarException{{Date: "2026-10-26", AvailableHours: hours(0)}}}
	known, unknown, err := Weight(partial, "2026-10-26", "2026-10-26")
	if err != nil || known != 0 || unknown != 0 {
		t.Fatalf("known zero override = (%v, %v, %v)", known, unknown, err)
	}
	known, unknown, err = Weight(partial, "2026-10-26", "2026-11-01")
	if err != nil || known != 0 || unknown != 6 {
		t.Fatalf("partial calendar = (%v, %v, %v); want (0, 6, nil)", known, unknown, err)
	}
}

// The 9,999-year fixture exceeds time.Duration's range and catches duration-based
// day counts. Its constant-size input also guards the intended closed-form path.
func TestCalendarWeightHandlesLongDateRangesAndLeapDays(t *testing.T) {
	schedule := ResourceCalendarSchedule{Weekdays: []*float64{hours(1), hours(1), hours(1), hours(1), hours(1), hours(1), hours(1)}}
	schedule.Exceptions = []ResourceCalendarException{
		{Date: "0001-01-01", AvailableHours: nil},
		{Date: "2000-02-29", AvailableHours: hours(0)},
		{Date: "9999-12-31", AvailableHours: hours(2)},
	}
	known, unknown, err := Weight(schedule, "0001-01-01", "9999-12-31")
	if err != nil || known != 3652058 || unknown != 1 {
		t.Fatalf("long interval = (%v, %v, %v); want (3652058, 1, nil)", known, unknown, err)
	}
	// Year 2000 is a leap year, unlike 1900.
	known, unknown, err = Weight(workweek(), "2000-02-28", "2000-03-05")
	if err != nil || known != 40 || unknown != 0 {
		t.Fatalf("leap week = (%v, %v, %v); want (40, 0, nil)", known, unknown, err)
	}
	known, unknown, err = Weight(workweek(), "2026-10-30", "2026-11-03")
	if err != nil || known != 24 || unknown != 0 {
		t.Fatalf("partial week = (%v, %v, %v); want (24, 0, nil)", known, unknown, err)
	}
}

// Invalid schedules must not produce apparently usable capacity. Quarter hours
// are exact binary fractions, so no rounding or tolerance may admit 0.1 hours.
func TestCalendarValidationRejectsInvalidHoursDatesAndExceptions(t *testing.T) {
	for _, value := range []float64{-0.25, 24.25, 0.1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		t.Run("invalid hours", func(t *testing.T) {
			schedule := workweek()
			schedule.Weekdays[0] = hours(value)
			if err := Validate(schedule); err == nil {
				t.Fatalf("accepted invalid hours %v", value)
			}
		})
	}
	for _, value := range []float64{0, 0.25, 7.75, 24} {
		schedule := workweek()
		schedule.Weekdays[0] = hours(value)
		if err := Validate(schedule); err != nil {
			t.Fatalf("rejected hours %v: %v", value, err)
		}
	}
	cases := []struct {
		name     string
		calendar ResourceCalendarSchedule
	}{
		{"wrong weekday length", ResourceCalendarSchedule{Weekdays: []*float64{hours(8)}}},
		{"empty explicit weekdays", ResourceCalendarSchedule{Weekdays: []*float64{}}},
		{"invalid date", ResourceCalendarSchedule{Exceptions: []ResourceCalendarException{{Date: "2026-02-29"}}}},
		{"noncanonical date", ResourceCalendarSchedule{Exceptions: []ResourceCalendarException{{Date: "2026-1-01"}}}},
		{"duplicate date", ResourceCalendarSchedule{Exceptions: []ResourceCalendarException{{Date: "2026-10-26"}, {Date: "2026-10-26"}}}},
		{"invalid exception hours", ResourceCalendarSchedule{Exceptions: []ResourceCalendarException{{Date: "2026-10-26", AvailableHours: hours(0.1)}}}},
		{"oversize note", ResourceCalendarSchedule{Exceptions: []ResourceCalendarException{{Date: "2026-10-26", Note: strings.Repeat("假", 201)}}}},
		{"invalid UTF-8 note", ResourceCalendarSchedule{Exceptions: []ResourceCalendarException{{Date: "2026-10-26", Note: string([]byte{0xff})}}}},
		{"too many exceptions", ResourceCalendarSchedule{Exceptions: make([]ResourceCalendarException, 367)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := Validate(tc.calendar); err == nil {
				t.Fatal("accepted invalid calendar")
			}
			if _, _, err := Weight(tc.calendar, "2026-10-26", "2026-11-01"); err == nil {
				t.Fatal("Weight accepted invalid calendar")
			}
			if _, err := Week(tc.calendar, "2026-10-26"); err == nil {
				t.Fatal("Week accepted invalid calendar")
			}
		})
	}
	valid := ResourceCalendarSchedule{Exceptions: []ResourceCalendarException{{Date: "2026-10-26", Note: strings.Repeat("假", 200) + "\n"}}}
	if err := Validate(valid); err == nil {
		t.Fatal("newline also counts toward the note limit")
	}
	valid.Exceptions[0].Note = "<plain text>\n" + strings.Repeat("假", 180)
	if err := Validate(valid); err != nil {
		t.Fatalf("valid plaintext note was rejected: %v", err)
	}
	valid.Exceptions[0].Note = strings.Repeat("假", 200)
	if err := Validate(valid); err != nil {
		t.Fatalf("valid 200-character note was rejected: %v", err)
	}
	// A full leap year of overrides is valid and can make a base-unknown year known.
	valid = ResourceCalendarSchedule{}
	for i := range 366 {
		date := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i).Format(time.DateOnly)
		valid.Exceptions = append(valid.Exceptions, ResourceCalendarException{Date: date, AvailableHours: hours(0.25)})
	}
	known, unknown, err := Weight(valid, "2000-01-01", "2000-12-31")
	if err != nil || known != 91.5 || unknown != 0 {
		t.Fatalf("366 distinct exceptions = (%v, %v, %v); want (91.5, 0, nil)", known, unknown, err)
	}
}

func TestCalendarMondayAndDateOnlyBoundaries(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"2026-10-26", "2026-10-26"},
		{"2026-11-01", "2026-10-26"},
		{"2027-01-03", "2026-12-28"},
		{"0001-01-01", "0001-01-01"},
	} {
		got, err := Monday(tc.input)
		if err != nil || got != tc.want {
			t.Errorf("Monday(%q) = (%q, %v); want %q", tc.input, got, err, tc.want)
		}
	}
	for _, date := range []string{"", "2026-02-29", "2026-10-26T00:00:00Z", "2026-1-01", "0000-01-01"} {
		if _, err := Monday(date); err == nil {
			t.Errorf("accepted date %q", date)
		}
	}
	if _, err := Week(workweek(), "2026-10-27"); err == nil {
		t.Fatal("Week accepted a non-Monday")
	}
	if _, err := Week(workweek(), "9999-12-27"); err == nil {
		t.Fatal("Week overflowed the supported date range")
	}
	if _, _, err := Weight(workweek(), "2026-11-01", "2026-10-26"); err == nil {
		t.Fatal("Weight accepted a reversed interval")
	}
}
