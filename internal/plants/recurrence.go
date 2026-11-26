package plants

import (
	"fmt"
	"strings"
	"time"

	"github.com/teambition/rrule-go"
)

const DateLayout = "2006-01-02"

func ParseRule(rule string, start time.Time) (*rrule.RRule, error) {
	rule = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rule), "RRULE:"))
	if rule == "" {
		return nil, fmt.Errorf("пустое правило")
	}
	if strings.Contains(strings.ToUpper(rule), "DTSTART") {
		return nil, fmt.Errorf("DTSTART задавать не нужно — началом считается следующая дата")
	}
	opt, err := rrule.StrToROption(rule)
	if err != nil {
		return nil, err
	}
	if opt.Freq > rrule.DAILY {
		return nil, fmt.Errorf("поддерживается периодичность от одного дня")
	}
	opt.Dtstart = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	return rrule.NewRRule(*opt)
}

func NextAfter(rule string, due, after time.Time) (next time.Time, ok bool, err error) {
	r, err := ParseRule(rule, due)
	if err != nil {
		return time.Time{}, false, err
	}
	n := r.After(time.Date(after.Year(), after.Month(), after.Day(), 23, 59, 59, 0, time.UTC), false)
	if n.IsZero() {
		return time.Time{}, false, nil
	}
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC), true, nil
}

func LocalToday(now time.Time, tz string) time.Time {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		loc = time.UTC
	}
	n := now.In(loc)
	return time.Date(n.Year(), n.Month(), n.Day(), 0, 0, 0, 0, time.UTC)
}
