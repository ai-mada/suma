// Package planningcalendar calculates date-only planning capacity. Availability
// is an input for estimated work distribution, never evidence of actual work.
package planningcalendar

import (
	"fmt"
	"math"
	"time"
	"unicode/utf8"
)

// ResourceCalendarSchedule holds Monday-through-Sunday availability. A nil
// weekday or nil exception value is unknown; an explicit zero is a day off.
// A nil Weekdays slice represents an unconfigured calendar, with all days unknown.
type ResourceCalendarSchedule struct {
	Weekdays   []*float64                  `json:"weekdays"`
	Exceptions []ResourceCalendarException `json:"exceptions"`
}

// ResourceCalendarException replaces the base availability on one exact date.
type ResourceCalendarException struct {
	Date           string   `json:"date"`
	AvailableHours *float64 `json:"available_hours"`
	Note           string   `json:"note"`
}

// ResourceCalendarDay retains both availability and its source. An explicit
// unknown exception keeps source "exception"; callers determine unknown from nil.
type ResourceCalendarDay struct {
	Date           string   `json:"date"`
	AvailableHours *float64 `json:"available_hours"`
	Source         string   `json:"source"`
}

// Validate checks the bounded calendar without changing or normalizing its data.
// Hours are exact quarter-hour values in [0, 24]; notes remain plain text.
func Validate(schedule ResourceCalendarSchedule) error {
	if schedule.Weekdays != nil && len(schedule.Weekdays) != 7 {
		return fmt.Errorf("weekdays must contain seven Monday-through-Sunday values")
	}
	for i, hours := range schedule.Weekdays {
		if !validHours(hours) {
			return fmt.Errorf("weekdays[%d] must be null or finite quarter hours between 0 and 24", i)
		}
	}
	if len(schedule.Exceptions) > 366 {
		return fmt.Errorf("calendar must contain at most 366 date exceptions")
	}
	seen := make(map[string]struct{}, len(schedule.Exceptions))
	for i, exception := range schedule.Exceptions {
		if _, err := parseDate(exception.Date); err != nil {
			return fmt.Errorf("exceptions[%d].date: %w", i, err)
		}
		if _, duplicate := seen[exception.Date]; duplicate {
			return fmt.Errorf("duplicate calendar exception date %s", exception.Date)
		}
		seen[exception.Date] = struct{}{}
		if !validHours(exception.AvailableHours) {
			return fmt.Errorf("exceptions[%d].available_hours must be null or finite quarter hours between 0 and 24", i)
		}
		if !utf8.ValidString(exception.Note) || utf8.RuneCountInString(exception.Note) > 200 {
			return fmt.Errorf("exceptions[%d].note must be valid UTF-8 with at most 200 characters", i)
		}
	}
	return nil
}

// Monday returns the Monday containing a valid YYYY-MM-DD date. All calendar
// arithmetic uses UTC date boundaries, independent of the process time zone.
func Monday(date string) (string, error) {
	day, err := parseDate(date)
	if err != nil {
		return "", err
	}
	return day.AddDate(0, 0, -mondayIndex(day)).Format(time.DateOnly), nil
}

// Week returns seven daily values for an exact Monday. Unknown base values use
// source "unknown"; any explicit override, including nil, uses "exception".
func Week(schedule ResourceCalendarSchedule, weekMonday string) ([]ResourceCalendarDay, error) {
	if err := Validate(schedule); err != nil {
		return nil, err
	}
	start, err := parseDate(weekMonday)
	if err != nil {
		return nil, err
	}
	if start.Weekday() != time.Monday {
		return nil, fmt.Errorf("week date must be a Monday")
	}
	if start.AddDate(0, 0, 6).Year() > 9999 {
		return nil, fmt.Errorf("week exceeds supported calendar dates")
	}
	exceptions := make(map[string]*float64, len(schedule.Exceptions))
	for _, exception := range schedule.Exceptions {
		exceptions[exception.Date] = exception.AvailableHours
	}
	days := make([]ResourceCalendarDay, 7)
	for i := range days {
		date := start.AddDate(0, 0, i).Format(time.DateOnly)
		available := baseHours(schedule, i)
		source := "base"
		if available == nil {
			source = "unknown"
		}
		if override, exists := exceptions[date]; exists {
			available, source = override, "exception"
		}
		days[i] = ResourceCalendarDay{Date: date, AvailableHours: copyHours(available), Source: source}
	}
	return days, nil
}

// Weight totals known available hours and unknown days over an inclusive task
// interval. A positive known total with unknown days is incomplete: callers must
// not use it to distribute an estimate. Known zero also cannot distribute a
// positive estimate. Neither condition permits an equal natural-day fallback.
//
// Work is bounded by seven weekdays and at most 366 exceptions, regardless of
// interval length. Unix-day subtraction avoids time.Duration's ~290-year limit.
func Weight(schedule ResourceCalendarSchedule, startDate, endDate string) (knownHours float64, unknownDays int, err error) {
	if err = Validate(schedule); err != nil {
		return 0, 0, err
	}
	start, err := parseDate(startDate)
	if err != nil {
		return 0, 0, err
	}
	end, err := parseDate(endDate)
	if err != nil {
		return 0, 0, err
	}
	if end.Before(start) {
		return 0, 0, fmt.Errorf("calendar interval end must not precede start")
	}
	dayCount := end.Unix()/86400 - start.Unix()/86400 + 1
	fullWeeks, remainder := dayCount/7, dayCount%7
	firstWeekday := mondayIndex(start)
	for weekday := range 7 {
		count := fullWeeks
		if int64((weekday-firstWeekday+7)%7) < remainder {
			count++
		}
		if hours := baseHours(schedule, weekday); hours != nil {
			knownHours += float64(count) * *hours
		} else {
			unknownDays += int(count)
		}
	}
	for _, exception := range schedule.Exceptions {
		// Validation has already established the canonical date and uniqueness.
		date, _ := parseDate(exception.Date)
		if date.Before(start) || date.After(end) {
			continue
		}
		if base := baseHours(schedule, mondayIndex(date)); base != nil {
			knownHours -= *base
		} else {
			unknownDays--
		}
		if exception.AvailableHours != nil {
			knownHours += *exception.AvailableHours
		} else {
			unknownDays++
		}
	}
	return knownHours, unknownDays, nil
}

func validHours(hours *float64) bool {
	if hours == nil {
		return true
	}
	return !math.IsNaN(*hours) && !math.IsInf(*hours, 0) && *hours >= 0 && *hours <= 24 && math.Trunc(*hours*4) == *hours*4
}

func parseDate(date string) (time.Time, error) {
	day, err := time.Parse(time.DateOnly, date)
	if err != nil || len(date) != 10 || day.Year() < 1 || day.Format(time.DateOnly) != date {
		return time.Time{}, fmt.Errorf("date must be a valid YYYY-MM-DD from 0001 through 9999")
	}
	return day, nil
}

func mondayIndex(day time.Time) int { return (int(day.Weekday()) + 6) % 7 }

func baseHours(schedule ResourceCalendarSchedule, weekday int) *float64 {
	if schedule.Weekdays == nil {
		return nil
	}
	return schedule.Weekdays[weekday]
}

func copyHours(hours *float64) *float64 {
	if hours == nil {
		return nil
	}
	value := *hours
	return &value
}
