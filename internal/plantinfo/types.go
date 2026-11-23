// Package plantinfo подбирает рекомендации по уходу для растения по внешнему справочнику Perenual:
// поиск вариантов по названию, детали выбранного вида, кэш в БД. Никаких «дефолтных» событий:
// если справочник не дал данных, об этом сказано явно (Coverage/Notice), а событий нет.
package plantinfo

import (
	"context"
	"errors"
)

// Candidate — вид (или сорт) из справочника, найденный по запросу.
type Candidate struct {
	ID               int      `json:"id"`
	CommonName       string   `json:"common_name"`
	Scientific       string   `json:"scientific"`
	OtherNames       []string `json:"other_names"`
	ImageURL         string   `json:"image_url"`
	DetailsAvailable bool     `json:"details_available"` // на бесплатном тарифе подробности есть только у видов с id 1–3000
}

// Facts — нормализованные данные о виде растения.
type Facts struct {
	SpeciesID       int      `json:"species_id"`
	CommonName      string   `json:"common_name"`
	Scientific      string   `json:"scientific"`
	ImageURL        string   `json:"image_url,omitempty"`
	Watering        string   `json:"watering,omitempty"`          // Frequent | Average | Minimum | None
	WateringMinDays int      `json:"watering_min_days,omitempty"` // из watering_general_benchmark
	WateringMaxDays int      `json:"watering_max_days,omitempty"`
	Sunlight        []string `json:"sunlight,omitempty"`
	PruningMonths   []int    `json:"pruning_months,omitempty"` // 1..12 (северное полушарие)
	CareLevel       string   `json:"care_level,omitempty"`
	Restricted      bool     `json:"restricted,omitempty"` // подробности по виду недоступны (тариф)
}

// Source — внешний справочник.
type Source interface {
	Search(ctx context.Context, query string) ([]Candidate, error)
	Details(ctx context.Context, id int) (*Facts, error)
}

var (
	ErrRateLimited = errors.New("лимит запросов к Perenual исчерпан")
	ErrUnavailable = errors.New("Perenual недоступен")
)

// SuggestedEvent совместим по полям с EventInput API. События строятся только из данных справочника.
type SuggestedEvent struct {
	Title          string   `json:"title"`
	Category       string   `json:"category"`
	Description    string   `json:"description"`
	RecurrenceKind string   `json:"recurrence_kind"`
	RecurrenceRule *string  `json:"recurrence_rule"`
	RecurrenceNote string   `json:"recurrence_note"`
	NextDueAt      string   `json:"next_due_at"`
	Channels       []string `json:"channels"`
	Source         string   `json:"source"` // всегда perenual
}

// Match — какой вид справочника сопоставлен растению и насколько точно.
type Match struct {
	SpeciesID   int      `json:"species_id"`
	CommonName  string   `json:"common_name"`
	Scientific  string   `json:"scientific"`
	ImageURL    string   `json:"image_url"`
	Sunlight    []string `json:"sunlight"`
	CareLevel   string   `json:"care_level"`
	Quality     string   `json:"quality"` // exact | approximate | chosen
	QualityNote string   `json:"quality_note"`
}

// Coverage — что справочник дал по каждому аспекту ухода.
type Coverage struct {
	Field     string `json:"field"` // watering | pruning | fertilizing
	Label     string `json:"label"`
	Available bool   `json:"available"`
	Detail    string `json:"detail"`
}

// Suggestion — результат подбора (ничего не сохраняет).
type Suggestion struct {
	Source      string           `json:"source"` // perenual | none
	Configured  bool             `json:"configured"`
	Match       *Match           `json:"match"`
	Coverage    []Coverage       `json:"coverage"`
	Notice      string           `json:"notice"`
	Attribution string           `json:"attribution"`
	Events      []SuggestedEvent `json:"events"`
}

// ReferenceSearch — что есть в справочнике по запросу.
type ReferenceSearch struct {
	Configured bool        `json:"configured"`
	Query      string      `json:"query"` // что реально искали (после перевода)
	QueryNote  string      `json:"query_note"`
	Results    []Candidate `json:"results"`
	Notice     string      `json:"notice"`
}
