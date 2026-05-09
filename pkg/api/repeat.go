package api

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

const dateLayout = "20060102"

type repeatRule struct {
	kind      string
	interval  int
	weekdays  []int
	monthDays []int
	months    []int
}

func parseDateYYYYMMDD(value string) (time.Time, error) {
	dt, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date")
	}
	if dt.Format(dateLayout) != value {
		return time.Time{}, fmt.Errorf("invalid date")
	}
	return dt, nil
}

func dayOnly(t time.Time) time.Time {
	dt, _ := parseDateYYYYMMDD(t.Format(dateLayout))
	return dt
}

func formatDateYYYYMMDD(t time.Time) string {
	return dayOnly(t).Format(dateLayout)
}

func validateRepeat(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	_, err := parseRepeatRule(value)
	if err != nil {
		return fmt.Errorf("invalid repeat")
	}
	return nil
}

func calcNextDate(now, date time.Time, repeat string) (string, error) {
	rule, err := parseRepeatRule(repeat)
	if err != nil {
		return "", fmt.Errorf("invalid repeat")
	}

	curr := dayOnly(date)
	now = dayOnly(now)

	for i := 0; i < 10000; i++ {
		next, err := rule.next(curr)
		if err != nil {
			return "", err
		}
		if next.After(now) {
			return formatDateYYYYMMDD(next), nil
		}
		curr = next
	}

	return "", fmt.Errorf("next date is not found")
}

func parseRepeatRule(raw string) (repeatRule, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return repeatRule{}, errors.New("empty repeat")
	}

	parts := strings.Fields(raw)
	switch parts[0] {
	case "y":
		if len(parts) != 1 {
			return repeatRule{}, errors.New("invalid y repeat")
		}
		return repeatRule{kind: "y"}, nil
	case "d":
		if len(parts) != 2 {
			return repeatRule{}, errors.New("invalid d repeat")
		}
		n, err := strconv.Atoi(parts[1])
		if err != nil || n < 1 || n > 400 {
			return repeatRule{}, errors.New("invalid d repeat")
		}
		return repeatRule{kind: "d", interval: n}, nil
	case "w":
		if len(parts) != 2 {
			return repeatRule{}, errors.New("invalid w repeat")
		}
		weekdays, err := parseIntList(parts[1], func(v int) bool {
			return v >= 1 && v <= 7
		})
		if err != nil || len(weekdays) == 0 {
			return repeatRule{}, errors.New("invalid w repeat")
		}
		return repeatRule{kind: "w", weekdays: weekdays}, nil
	case "m":
		if len(parts) != 2 && len(parts) != 3 {
			return repeatRule{}, errors.New("invalid m repeat")
		}

		monthDays, err := parseIntList(parts[1], func(v int) bool {
			return (v >= 1 && v <= 31) || v == -1 || v == -2
		})
		if err != nil || len(monthDays) == 0 {
			return repeatRule{}, errors.New("invalid m repeat")
		}

		var months []int
		if len(parts) == 3 {
			months, err = parseIntList(parts[2], func(v int) bool {
				return v >= 1 && v <= 12
			})
			if err != nil || len(months) == 0 {
				return repeatRule{}, errors.New("invalid m repeat")
			}
		}

		return repeatRule{
			kind:      "m",
			monthDays: monthDays,
			months:    months,
		}, nil
	default:
		return repeatRule{}, errors.New("unknown repeat rule")
	}
}

func parseIntList(raw string, valid func(int) bool) ([]int, error) {
	parts := strings.Split(raw, ",")
	if len(parts) == 0 {
		return nil, errors.New("empty list")
	}

	set := make(map[int]struct{}, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			return nil, errors.New("invalid list")
		}
		v, err := strconv.Atoi(p)
		if err != nil || !valid(v) {
			return nil, errors.New("invalid list")
		}
		set[v] = struct{}{}
	}

	values := make([]int, 0, len(set))
	for v := range set {
		values = append(values, v)
	}
	sort.Ints(values)
	return values, nil
}

func (r repeatRule) next(from time.Time) (time.Time, error) {
	from = dayOnly(from)
	switch r.kind {
	case "y":
		return from.AddDate(1, 0, 0), nil
	case "d":
		return from.AddDate(0, 0, r.interval), nil
	case "w":
		return r.nextWeekday(from), nil
	case "m":
		return r.nextMonthDay(from)
	default:
		return time.Time{}, errors.New("unsupported repeat rule")
	}
}

func (r repeatRule) nextWeekday(from time.Time) time.Time {
	allowed := make(map[int]struct{}, len(r.weekdays))
	for _, d := range r.weekdays {
		allowed[d] = struct{}{}
	}

	next := from.AddDate(0, 0, 1)
	for i := 0; i < 3700; i++ {
		if _, ok := allowed[toWeekdayNumber(next.Weekday())]; ok {
			return next
		}
		next = next.AddDate(0, 0, 1)
	}
	return from
}

func toWeekdayNumber(w time.Weekday) int {
	if w == time.Sunday {
		return 7
	}
	return int(w)
}

func (r repeatRule) nextMonthDay(from time.Time) (time.Time, error) {
	start := from.AddDate(0, 0, 1)
	startYear, startMonth, _ := start.Date()

	monthsAllowed := make(map[int]struct{}, len(r.months))
	for _, m := range r.months {
		monthsAllowed[m] = struct{}{}
	}

	for offset := 0; offset < 2400; offset++ {
		monthNumber := int(startMonth) + offset
		year := startYear + (monthNumber-1)/12
		month := time.Month((monthNumber-1)%12 + 1)

		if len(monthsAllowed) > 0 {
			if _, ok := monthsAllowed[int(month)]; !ok {
				continue
			}
		}

		daysInMonth := daysIn(year, month)
		days := r.expandMonthDays(daysInMonth)
		for _, day := range days {
			candidate := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
			if candidate.After(from) {
				return candidate, nil
			}
		}
	}

	return time.Time{}, errors.New("next date is not found")
}

func (r repeatRule) expandMonthDays(daysInMonth int) []int {
	result := make([]int, 0, len(r.monthDays))
	seen := make(map[int]struct{}, len(r.monthDays))

	for _, spec := range r.monthDays {
		day := spec
		switch spec {
		case -1:
			day = daysInMonth
		case -2:
			day = daysInMonth - 1
		}

		if day < 1 || day > daysInMonth {
			continue
		}
		if _, ok := seen[day]; ok {
			continue
		}
		seen[day] = struct{}{}
		result = append(result, day)
	}

	sort.Ints(result)
	return result
}

func daysIn(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}
