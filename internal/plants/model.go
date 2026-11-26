package plants

import "time"

type Plant struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Emoji       string    `json:"emoji"`
	EventsCount int       `json:"events_count"`
	NextDueAt   *string   `json:"next_due_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type PlantDetail struct {
	Plant
	Events []CareEvent `json:"events"`
}

type CareEvent struct {
	ID             int64      `json:"id"`
	PlantID        int64      `json:"plant_id"`
	Title          string     `json:"title"`
	Category       string     `json:"category"`
	Description    string     `json:"description"`
	RecurrenceKind string     `json:"recurrence_kind"`
	RecurrenceRule *string    `json:"recurrence_rule"`
	RecurrenceNote string     `json:"recurrence_note"`
	NextDueAt      string     `json:"next_due_at"`
	Channels       []string   `json:"channels"`
	Active         bool       `json:"active"`
	CompletedAt    *time.Time `json:"completed_at"`
	CanUndo        bool       `json:"can_undo"`
}

type UpcomingEvent struct {
	Event CareEvent `json:"event"`
	Plant struct {
		ID    int64  `json:"id"`
		Name  string `json:"name"`
		Emoji string `json:"emoji"`
	} `json:"plant"`
}

type PlantInput struct {
	Name  string `json:"name"`
	Emoji string `json:"emoji"`
}

type PlantPatch struct {
	Name  *string `json:"name"`
	Emoji *string `json:"emoji"`
}

type EventInput struct {
	Title          string   `json:"title"`
	Category       string   `json:"category"`
	Description    string   `json:"description"`
	RecurrenceKind string   `json:"recurrence_kind"`
	RecurrenceRule *string  `json:"recurrence_rule"`
	RecurrenceNote string   `json:"recurrence_note"`
	NextDueAt      string   `json:"next_due_at"`
	Channels       []string `json:"channels"`
	Active         *bool    `json:"active"`
}

type EventPatch struct {
	Title          *string  `json:"title"`
	Category       *string  `json:"category"`
	Description    *string  `json:"description"`
	RecurrenceKind *string  `json:"recurrence_kind"`
	RecurrenceRule *string  `json:"recurrence_rule"`
	RecurrenceNote *string  `json:"recurrence_note"`
	NextDueAt      *string  `json:"next_due_at"`
	Channels       []string `json:"channels"`
	Active         *bool    `json:"active"`
	Reopen         *bool    `json:"reopen"`
}

var Categories = []string{
	"Полив", "Влагозарядковый полив", "Обрезка санитарная", "Обрезка формирующая", "Подкормка",
	"Обработка от болезней и вредителей", "Побелка стволов", "Защита от грызунов", "Удаление поросли",
	"Прореживание завязей", "Уборка листвы и падалицы", "Перекопка приствольного круга", "Сбор урожая", "Другое",
}

var Channels = []string{"max", "vk", "telegram"}
