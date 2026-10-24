// Package automation implements bounded, numeric five-field cron schedules.
package automation

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

// The fixed search budget bounds both impossible dates and sparse schedules.
const SearchDays = 8 * 366

type field struct {
	allowed  [60]bool
	wildcard bool
}
type Schedule struct {
	fields   [5]field
	location *time.Location
}

func Parse(expression, zone string) (*Schedule, error) {
	if len(expression) > 256 || len(zone) > 100 || zone == "" || zone == "Local" {
		return nil, fmt.Errorf("invalid cron or IANA timezone")
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		return nil, fmt.Errorf("invalid IANA timezone: %w", err)
	}
	parts := strings.Fields(expression)
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron requires five numeric fields")
	}
	s := &Schedule{location: loc}
	mins, maxs := [5]int{0, 0, 1, 1, 0}, [5]int{59, 23, 31, 12, 7}
	for i, part := range parts {
		if len(part) > 100 {
			return nil, fmt.Errorf("cron field too long")
		}
		items := strings.Split(part, ",")
		if len(items) > 60 {
			return nil, fmt.Errorf("cron list too long")
		}
		for _, item := range items {
			step := 1
			base := item
			hasStep := false
			if strings.Contains(item, "/") {
				hasStep = true
				bits := strings.Split(item, "/")
				if len(bits) != 2 {
					return nil, fmt.Errorf("invalid cron step")
				}
				base = bits[0]
				step, err = strictNumber(bits[1])
				if err != nil || step < 1 || step > maxs[i]-mins[i]+1 {
					return nil, fmt.Errorf("invalid cron step")
				}
			}
			lo, hi := mins[i], maxs[i]
			// The unrestricted marker belongs to each list item. A redundant
			// literal before '*' must not turn a weekday restriction into OR.
			if base == "*" {
				s.fields[i].wildcard = true
			}
			if base != "*" {
				bounds := strings.Split(base, "-")
				if len(bounds) > 2 {
					return nil, fmt.Errorf("invalid cron range")
				}
				lo, err = strictNumber(bounds[0])
				if err != nil {
					return nil, err
				}
				hi = lo
				if len(bounds) == 2 {
					hi, err = strictNumber(bounds[1])
					if err != nil {
						return nil, err
					}
				} else if hasStep {
					hi = maxs[i]
				}
			}
			if lo < mins[i] || hi > maxs[i] || lo > hi {
				return nil, fmt.Errorf("cron field outside range")
			}
			for n := lo; n <= hi; n += step {
				if i == 4 && n == 7 {
					s.fields[i].allowed[0] = true
				} else {
					s.fields[i].allowed[n] = true
				}
			}
		}
	}
	// When weekday is unrestricted the selected months must contain at least
	// one selected day, including February 29 in a leap year.
	if s.fields[4].wildcard {
		reachable := false
		for month := 1; month <= 12; month++ {
			if !s.fields[3].allowed[month] {
				continue
			}
			maxDay := time.Date(2000, time.Month(month)+1, 0, 0, 0, 0, 0, time.UTC).Day()
			for day := 1; day <= maxDay; day++ {
				reachable = reachable || s.fields[2].allowed[day]
			}
		}
		if !reachable {
			return nil, fmt.Errorf("cron calendar has no possible date")
		}
	}
	return s, nil
}
func strictNumber(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty cron number")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("cron fields must be numeric")
		}
	}
	return strconv.Atoi(s)
}

// Next returns a UTC instant strictly after after. Civil minutes are checked
// against every actual UTC offset, preserving both sides of a DST fold.
func (s *Schedule) Next(after time.Time) (time.Time, error) {
	return s.NextContext(context.Background(), after)
}
func (s *Schedule) Previous(at time.Time) (time.Time, error) {
	return s.PreviousContext(context.Background(), at)
}
func (s *Schedule) NextContext(ctx context.Context, after time.Time) (time.Time, error) {
	return s.search(ctx, after.Truncate(time.Minute).Add(time.Minute), 1)
}
func (s *Schedule) PreviousContext(ctx context.Context, at time.Time) (time.Time, error) {
	return s.search(ctx, at.Truncate(time.Minute), -1)
}
func (s *Schedule) search(ctx context.Context, bound time.Time, direction int) (time.Time, error) {
	local := bound.In(s.location)
	// A UTC date is used only as a Gregorian calendar counter. Constructing the
	// counter in the target zone could normalize away an entirely missing day.
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	for n := 0; n < SearchDays; n++ {
		if err := ctx.Err(); err != nil {
			return time.Time{}, err
		}
		f := s.fields
		dom, dow := f[2].allowed[date.Day()], f[4].allowed[int(date.Weekday())]
		dayMatch := dom || dow
		if f[2].wildcard || f[4].wildcard {
			dayMatch = dom && dow
		}
		if f[3].allowed[int(date.Month())] && dayMatch {
			// Enumerate offset periods touching this civil day. ZoneBounds gives
			// exact transition instants, including non-hour and 24-hour changes.
			offsets := map[int]bool{}
			start := date.Add(-48 * time.Hour)
			end := date.Add(72 * time.Hour)
			for probe := start; probe.Before(end); {
				z := probe.In(s.location)
				_, offset := z.Zone()
				offsets[offset] = true
				_, until := z.ZoneBounds()
				if until.IsZero() || !until.Before(end) {
					break
				}
				if !until.After(probe) {
					return time.Time{}, fmt.Errorf("invalid timezone transition")
				}
				probe = until
			}
			var best time.Time
			for hour := 0; hour < 24; hour++ {
				if !f[1].allowed[hour] {
					continue
				}
				for minute := 0; minute < 60; minute++ {
					if !f[0].allowed[minute] {
						continue
					}
					for offset := range offsets {
						candidate := date.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute - time.Duration(offset)*time.Second)
						actual := candidate.In(s.location)
						if actual.Year() != date.Year() || actual.Month() != date.Month() || actual.Day() != date.Day() || actual.Hour() != hour || actual.Minute() != minute || actual.Second() != 0 {
							continue
						}
						if direction > 0 {
							if !candidate.Before(bound) && (best.IsZero() || candidate.Before(best)) {
								best = candidate
							}
						} else if !candidate.After(bound) && (best.IsZero() || candidate.After(best)) {
							best = candidate
						}
					}
				}
			}
			if !best.IsZero() {
				return best.UTC(), nil
			}
		}
		date = date.AddDate(0, 0, direction)
	}
	return time.Time{}, fmt.Errorf("cron has no occurrence within bounded eight-year search")
}
func (s *Schedule) Local(at time.Time) string { return at.In(s.location).Format(time.RFC3339) }
