package plants

import (
	"testing"
	"time"
)

func d(s string) time.Time { t, _ := time.Parse(DateLayout, s); return t }

func TestNextAfter(t *testing.T) {
	cases := []struct {
		rule, due, after, want string
		ok                     bool
	}{
		{"FREQ=DAILY", "2026-11-23", "2026-11-23", "2026-11-24", true},
		{"FREQ=WEEKLY", "2026-11-23", "2026-11-23", "2026-11-30", true},
		{"FREQ=WEEKLY", "2026-11-23", "2026-12-01", "2026-12-07", true},
		{"FREQ=WEEKLY;INTERVAL=2", "2026-11-23", "2026-11-23", "2026-12-07", true},
		{"FREQ=MONTHLY", "2026-01-31", "2026-01-31", "2026-03-31", true},
		{"FREQ=YEARLY", "2026-03-05", "2026-03-05", "2027-03-05", true},
		{"FREQ=DAILY;COUNT=1", "2026-11-23", "2026-11-23", "", false},
	}
	for _, c := range cases {
		got, ok, err := NextAfter(c.rule, d(c.due), d(c.after))
		if err != nil {
			t.Fatalf("%s: %v", c.rule, err)
		}
		if ok != c.ok || (ok && got.Format(DateLayout) != c.want) {
			t.Errorf("%s due=%s after=%s: got %v ok=%v, want %s ok=%v", c.rule, c.due, c.after, got.Format(DateLayout), ok, c.want, c.ok)
		}
	}
}

func TestParseRuleRejects(t *testing.T) {
	for _, bad := range []string{"", "FREQ=HOURLY", "FREQ=SECONDLY", "garbage", "DTSTART:20260101T000000Z\nRRULE:FREQ=DAILY"} {
		if _, err := ParseRule(bad, d("2026-11-23")); err == nil {
			t.Errorf("правило %q должно быть отклонено", bad)
		}
	}
}

func TestLocalToday(t *testing.T) {
	now := time.Date(2026, 11, 23, 20, 0, 0, 0, time.UTC)
	if got := LocalToday(now, "Europe/Moscow").Format(DateLayout); got != "2026-11-23" {
		t.Errorf("Москва: %s", got)
	}
	if got := LocalToday(now, "Asia/Vladivostok").Format(DateLayout); got != "2026-11-24" {
		t.Errorf("Владивосток: %s", got)
	}
}
