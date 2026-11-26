package plantinfo

import (
	"context"
	"errors"
)

type Candidate struct {
	ID               int      `json:"id"`
	CommonName       string   `json:"common_name"`
	Scientific       string   `json:"scientific"`
	OtherNames       []string `json:"other_names"`
	ImageURL         string   `json:"image_url"`
	DetailsAvailable bool     `json:"details_available"`
}

type Facts struct {
	SpeciesID       int      `json:"species_id"`
	CommonName      string   `json:"common_name"`
	Scientific      string   `json:"scientific"`
	ImageURL        string   `json:"image_url,omitempty"`
	Watering        string   `json:"watering,omitempty"`
	WateringMinDays int      `json:"watering_min_days,omitempty"`
	WateringMaxDays int      `json:"watering_max_days,omitempty"`
	Sunlight        []string `json:"sunlight,omitempty"`
	PruningMonths   []int    `json:"pruning_months,omitempty"`
	CareLevel       string   `json:"care_level,omitempty"`
	Restricted      bool     `json:"restricted,omitempty"`
}

type Source interface {
	Search(ctx context.Context, query string) ([]Candidate, error)
	Details(ctx context.Context, id int) (*Facts, error)
}

var (
	ErrRateLimited = errors.New("лимит запросов к Perenual исчерпан")
	ErrUnavailable = errors.New("Perenual недоступен")
)

type SuggestedEvent struct {
	Title          string   `json:"title"`
	Category       string   `json:"category"`
	Description    string   `json:"description"`
	RecurrenceKind string   `json:"recurrence_kind"`
	RecurrenceRule *string  `json:"recurrence_rule"`
	RecurrenceNote string   `json:"recurrence_note"`
	NextDueAt      string   `json:"next_due_at"`
	Channels       []string `json:"channels"`
	Source         string   `json:"source"`
}

type Match struct {
	SpeciesID   int      `json:"species_id"`
	CommonName  string   `json:"common_name"`
	Scientific  string   `json:"scientific"`
	ImageURL    string   `json:"image_url"`
	Sunlight    []string `json:"sunlight"`
	CareLevel   string   `json:"care_level"`
	Quality     string   `json:"quality"`
	QualityNote string   `json:"quality_note"`
}

type Coverage struct {
	Field     string `json:"field"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	Detail    string `json:"detail"`
}

type Suggestion struct {
	Source      string           `json:"source"`
	Configured  bool             `json:"configured"`
	Match       *Match           `json:"match"`
	Coverage    []Coverage       `json:"coverage"`
	Notice      string           `json:"notice"`
	Attribution string           `json:"attribution"`
	Events      []SuggestedEvent `json:"events"`
}

type ReferenceSearch struct {
	Configured bool        `json:"configured"`
	Query      string      `json:"query"`
	QueryNote  string      `json:"query_note"`
	Results    []Candidate `json:"results"`
	Notice     string      `json:"notice"`
}
