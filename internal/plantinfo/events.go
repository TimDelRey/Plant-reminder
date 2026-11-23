package plantinfo

import (
	"fmt"
	"strings"
	"time"
)

const dateLayout = "2006-01-02"

var monthsRU = []string{"", "январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"}

var sunlightRU = map[string]string{
	"full sun": "яркое солнце", "part shade": "полутень", "part sun": "полутень", "partial shade": "полутень",
	"full shade": "тень", "filtered shade": "рассеянный свет", "sun-part shade": "солнце или полутень",
	"deep shade": "глубокая тень", "part sun/part shade": "полутень",
}

var wateringRU = map[string]string{"Frequent": "частый", "Average": "умеренный", "Minimum": "редкий"}

func sunlightLabels(in []string) []string {
	out := []string{} // не nil: в JSON должен быть [], а не null
	seen := map[string]bool{}
	for _, s := range in {
		l := strings.ToLower(strings.TrimSpace(s))
		ru, ok := sunlightRU[l]
		if !ok {
			ru = l
		}
		if !seen[ru] {
			seen[ru] = true
			out = append(out, ru)
		}
	}
	return out
}

func ptr(s string) *string { return &s }

func plural(n int, one, few, many string) string {
	n10, n100 := n%10, n%100
	switch {
	case n100 >= 11 && n100 <= 14:
		return many
	case n10 == 1:
		return one
	case n10 >= 2 && n10 <= 4:
		return few
	}
	return many
}

// intervalRule — правило RFC 5545 для «раз в n дней».
func intervalRule(n int) (rule, note string) {
	switch {
	case n == 1:
		return "FREQ=DAILY", "Каждый день"
	case n == 7:
		return "FREQ=WEEKLY", "Раз в неделю"
	case n%7 == 0 && n <= 28:
		w := n / 7
		return fmt.Sprintf("FREQ=WEEKLY;INTERVAL=%d", w), fmt.Sprintf("Раз в %d %s", w, plural(w, "неделю", "недели", "недель"))
	}
	return fmt.Sprintf("FREQ=DAILY;INTERVAL=%d", n), fmt.Sprintf("Раз в %d %s", n, plural(n, "день", "дня", "дней"))
}

func date(t time.Time) string { return t.Format(dateLayout) }

// build строит предложение ухода ТОЛЬКО из данных справочника и честно показывает, что найдено, а что нет.
func build(f *Facts, today time.Time, channels []string) ([]SuggestedEvent, []Coverage) {
	if channels == nil {
		channels = []string{}
	}
	events := []SuggestedEvent{}
	cov := []Coverage{}
	const noData = "нет данных в справочнике"
	restricted := "подробности по этому виду недоступны на текущем тарифе Perenual"

	// полив
	days := wateringDays(f)
	switch {
	case days > 0:
		rule, note := intervalRule(days)
		desc := "Поливать по подсыханию верхнего слоя почвы."
		detail := ""
		if f.WateringMinDays > 0 && f.WateringMaxDays > f.WateringMinDays {
			detail = fmt.Sprintf("раз в %d–%d дн.", f.WateringMinDays, f.WateringMaxDays)
			desc += " По данным Perenual — раз в " + fmt.Sprintf("%d–%d", f.WateringMinDays, f.WateringMaxDays) + " дн."
		} else if f.WateringMinDays > 0 {
			detail = fmt.Sprintf("раз в %d дн.", f.WateringMinDays)
			desc += fmt.Sprintf(" По данным Perenual — раз в %d дн.", f.WateringMinDays)
		} else {
			detail = "потребность во влаге: " + wateringRU[f.Watering] + " (ориентировочный интервал)"
			desc += " Потребность во влаге по данным Perenual: " + wateringRU[f.Watering] + "."
		}
		events = append(events, SuggestedEvent{Title: "Полив", Category: "Полив", Description: desc, RecurrenceKind: "rrule",
			RecurrenceRule: &rule, RecurrenceNote: note, NextDueAt: date(today.AddDate(0, 0, days)), Channels: channels, Source: "perenual"})
		cov = append(cov, Coverage{Field: "watering", Label: "Полив", Available: true, Detail: detail})
	case f.Watering == "None":
		cov = append(cov, Coverage{Field: "watering", Label: "Полив", Available: false, Detail: "справочник: растение не требует полива"})
	case f.Restricted:
		cov = append(cov, Coverage{Field: "watering", Label: "Полив", Available: false, Detail: restricted})
	default:
		cov = append(cov, Coverage{Field: "watering", Label: "Полив", Available: false, Detail: noData})
	}

	// обрезка
	switch {
	case len(f.PruningMonths) > 0:
		first := f.PruningMonths[0]
		due := time.Date(today.Year(), time.Month(first), 15, 0, 0, 0, 0, time.UTC)
		if !due.After(today) {
			due = due.AddDate(1, 0, 0)
		}
		var names []string
		for _, m := range f.PruningMonths {
			names = append(names, monthsRU[m])
		}
		list := strings.Join(names, ", ")
		events = append(events, SuggestedEvent{Title: "Обрезка", Category: "Обрезка санитарная",
			Description:    "Удалить повреждённые и сухие ветви, при необходимости сформировать крону. Время обрезки по данным Perenual: " + list + ".",
			RecurrenceKind: "rrule", RecurrenceRule: ptr("FREQ=YEARLY"), RecurrenceNote: "Ежегодно, " + strings.Join(names, " / "),
			NextDueAt: date(due), Channels: channels, Source: "perenual"})
		cov = append(cov, Coverage{Field: "pruning", Label: "Обрезка", Available: true, Detail: list})
	case f.Restricted:
		cov = append(cov, Coverage{Field: "pruning", Label: "Обрезка", Available: false, Detail: restricted})
	default:
		cov = append(cov, Coverage{Field: "pruning", Label: "Обрезка", Available: false, Detail: noData})
	}

	// подкормка: в Perenual таких данных нет вообще
	cov = append(cov, Coverage{Field: "fertilizing", Label: "Подкормка", Available: false,
		Detail: "в Perenual нет данных о подкормке — добавьте событие вручную"})
	return events, cov
}

// wateringDays — интервал полива в днях по данным Perenual (0, если данных нет).
func wateringDays(f *Facts) int {
	if f.WateringMinDays > 0 {
		n := (f.WateringMinDays + f.WateringMaxDays + 1) / 2
		return min(max(n, 1), 60)
	}
	switch f.Watering {
	case "Frequent":
		return 3
	case "Average":
		return 7
	case "Minimum":
		return 14
	}
	return 0
}
