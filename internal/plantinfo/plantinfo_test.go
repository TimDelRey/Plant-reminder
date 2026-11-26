package plantinfo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"plantcare/internal/testutil"
)

var today = time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)

func fakePerenual(t *testing.T, calls *atomic.Int32, search, details string, status int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("key") != "k" {
			http.Error(w, "no key", 401)
			return
		}
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v2/species-list"):
			_, _ = w.Write([]byte(search))
		case strings.HasPrefix(r.URL.Path, "/api/v2/species/details/"):
			_, _ = w.Write([]byte(details))
		default:
			http.NotFound(w, r)
		}
	}))
}

const searchPlum = `{"data":[{"id":5000,"common_name":"Premium Only","scientific_name":["X y"]},
 {"id":42,"common_name":"Plum","scientific_name":["Prunus domestica"],"other_name":["Common Plum"],"default_image":{"medium_url":"https://img/plum.jpg"}},
 {"id":43,"common_name":"Mirabelle Plum","scientific_name":["Prunus domestica subsp. syriaca"]}]}`

const detailsPlum = `{"common_name":"Plum","scientific_name":["Prunus domestica"],"watering":"Average",
 "watering_general_benchmark":{"value":"\"5-7\"","unit":"days"},
 "sunlight":["full sun","Part shade"],"pruning_month":["March","April"],"care_level":"Medium"}`

func TestClientSearchOrdersByAvailableDetails(t *testing.T) {
	var calls atomic.Int32
	srv := fakePerenual(t, &calls, searchPlum, detailsPlum, 0)
	defer srv.Close()
	c := NewClient("k", srv.URL)
	cands, err := c.Search(context.Background(), "plum")
	if err != nil || len(cands) != 3 {
		t.Fatalf("%+v %v", cands, err)
	}
	if cands[0].ID != 42 || !cands[0].DetailsAvailable || cands[2].ID != 5000 || cands[2].DetailsAvailable {
		t.Errorf("сначала должны идти виды с доступными подробностями: %+v", cands)
	}
	if cands[0].ImageURL != "https://img/plum.jpg" || cands[0].OtherNames[0] != "Common Plum" || cands[1].OtherNames == nil {
		t.Errorf("поля: %+v", cands)
	}
}

func TestClientDetailsParsesAndHandlesRestrictions(t *testing.T) {
	var calls atomic.Int32
	srv := fakePerenual(t, &calls, "", detailsPlum, 0)
	f, err := NewClient("k", srv.URL).Details(context.Background(), 42)
	srv.Close()
	if err != nil || f.Watering != "Average" || f.WateringMinDays != 5 || f.WateringMaxDays != 7 || len(f.PruningMonths) != 2 || f.Restricted {
		t.Fatalf("%+v %v", f, err)
	}

	restr := `{"watering":"Upgrade Plans To Premium/Supreme - https://perenual.com/subscription-api-pricing. I'm sorry",
	 "watering_general_benchmark":{"value":"Upgrade Plans To Premium","unit":"days"},"sunlight":"Upgrade Plans","pruning_month":"Upgrade Plans"}`
	srv = fakePerenual(t, &calls, "", restr, 0)
	f, _ = NewClient("k", srv.URL).Details(context.Background(), 42)
	srv.Close()
	if f.Watering != "" || f.WateringMinDays != 0 || len(f.PruningMonths) != 0 || len(f.Sunlight) != 0 {
		t.Errorf("заглушки должны игнорироваться: %+v", f)
	}

	srv = fakePerenual(t, &calls, "", "", 403)
	f, err = NewClient("k", srv.URL).Details(context.Background(), 5000)
	srv.Close()
	if err != nil || !f.Restricted {
		t.Errorf("403 → Restricted: %+v %v", f, err)
	}

	srv = fakePerenual(t, &calls, "", "", 429)
	_, err = NewClient("k", srv.URL).Details(context.Background(), 1)
	srv.Close()
	if err != ErrRateLimited {
		t.Errorf("429 → ErrRateLimited, got %v", err)
	}
}

type fakeSource struct {
	search       map[string][]Candidate
	details      map[int]*Facts
	err          error
	searchCalls  int
	detailsCalls int
}

func (f *fakeSource) Search(_ context.Context, q string) ([]Candidate, error) {
	f.searchCalls++
	return f.search[q], f.err
}
func (f *fakeSource) Details(_ context.Context, id int) (*Facts, error) {
	f.detailsCalls++
	if f.err != nil {
		return nil, f.err
	}
	if d, ok := f.details[id]; ok {
		return d, nil
	}
	return &Facts{SpeciesID: id, Restricted: true}, nil
}

func plumSource() *fakeSource {
	return &fakeSource{
		search: map[string][]Candidate{
			"plum":         {{ID: 42, CommonName: "Plum", Scientific: "Prunus domestica", OtherNames: []string{}, DetailsAvailable: true}, {ID: 43, CommonName: "Mirabelle Plum", Scientific: "Prunus syriaca", DetailsAvailable: true}, {ID: 4000, CommonName: "Burbank Plum", Scientific: "Prunus salicina 'Burbank'"}},
			"burbank plum": {{ID: 4000, CommonName: "Burbank Plum", Scientific: "Prunus salicina 'Burbank'"}},
		},
		details: map[int]*Facts{
			42: {SpeciesID: 42, CommonName: "Plum", Scientific: "Prunus domestica", Watering: "Average", WateringMinDays: 7, WateringMaxDays: 10, PruningMonths: []int{2, 3}, Sunlight: []string{"full sun"}},
			43: {SpeciesID: 43, CommonName: "Mirabelle Plum", Scientific: "Prunus syriaca"},
		},
	}
}

func TestSuggestExactMatchBuildsOnlyReferenceEvents(t *testing.T) {
	s := &Service{Source: plumSource()}
	sug, err := s.Suggest(context.Background(), "plum", 0, today, []string{"telegram"})
	if err != nil || sug.Source != "perenual" || sug.Match.Quality != "exact" || sug.Match.Scientific != "Prunus domestica" {
		t.Fatalf("%+v %v", sug, err)
	}
	if len(sug.Events) != 2 {
		t.Fatalf("события должны быть только из справочника (полив, обрезка), без дефолтной подкормки: %+v", sug.Events)
	}
	for _, e := range sug.Events {
		if e.Source != "perenual" || e.Channels[0] != "telegram" {
			t.Errorf("%+v", e)
		}
	}
	w := sug.Events[0]
	if *w.RecurrenceRule != "FREQ=DAILY;INTERVAL=9" || w.NextDueAt != "2026-12-02" {
		t.Errorf("полив: %+v", w)
	}
	if p := sug.Events[1]; p.NextDueAt != "2027-02-15" || *p.RecurrenceRule != "FREQ=YEARLY" {
		t.Errorf("обрезка: %+v", p)
	}
	byField := map[string]Coverage{}
	for _, c := range sug.Coverage {
		byField[c.Field] = c
	}
	if !byField["watering"].Available || !byField["pruning"].Available || byField["fertilizing"].Available || !strings.Contains(byField["fertilizing"].Detail, "нет данных") {
		t.Errorf("coverage: %+v", sug.Coverage)
	}
}

func TestVarietyIsApproximateAndExplained(t *testing.T) {
	s := &Service{Source: plumSource()}
	sr, _ := s.Search(context.Background(), "Слива Бербанк")
	if sr.Query != "plum" || !strings.Contains(strings.ToLower(sr.QueryNote), "бербанк") || len(sr.Results) != 3 {
		t.Fatalf("search: %+v", sr)
	}
	sug, _ := s.Suggest(context.Background(), "Слива Бербанк", 0, today, nil)
	if sug.Match == nil || sug.Match.Quality != "approximate" || !strings.Contains(sug.Match.QualityNote, "Точного совпадения") || !strings.Contains(sug.Match.QualityNote, "Plum") {
		t.Fatalf("match: %+v", sug.Match)
	}

	sug, _ = s.Suggest(context.Background(), "Слива Бербанк", 43, today, nil)
	if sug.Match.Quality != "chosen" || sug.Match.CommonName != "Mirabelle Plum" {
		t.Errorf("chosen: %+v", sug.Match)
	}
}

func TestNoDataMeansNoEventsAndExplicitExplanation(t *testing.T) {
	s := &Service{Source: plumSource()}

	sug, _ := s.Suggest(context.Background(), "x", 43, today, nil)
	if sug.Source != "perenual" || len(sug.Events) != 0 || sug.Notice == "" {
		t.Fatalf("%+v", sug)
	}
	for _, c := range sug.Coverage {
		if c.Available {
			t.Errorf("%+v не должно быть доступно", c)
		}
	}

	sug, _ = s.Suggest(context.Background(), "x", 4000, today, nil)
	if len(sug.Events) != 0 || !strings.Contains(sug.Coverage[0].Detail, "тарифе") {
		t.Errorf("restricted: %+v", sug)
	}

	sug, _ = s.Suggest(context.Background(), "Zzzz", 0, today, nil)
	if sug.Source != "none" || len(sug.Events) != 0 || sug.Match != nil || !strings.Contains(sug.Notice, "ничего") && !strings.Contains(sug.Notice, "нет ничего") {
		t.Errorf("not found: %+v", sug)
	}

	sug, _ = s.Suggest(context.Background(), "Неведомая трава", 0, today, nil)
	if sug.Source != "none" || !strings.Contains(sug.Notice, "перевести") {
		t.Errorf("untranslatable: %+v", sug)
	}
}

func TestNotConfiguredAndOutages(t *testing.T) {
	sug, _ := (&Service{}).Suggest(context.Background(), "Слива", 0, today, nil)
	if sug.Source != "none" || sug.Configured || len(sug.Events) != 0 || !strings.Contains(sug.Notice, "PERENUAL_API_KEY") {
		t.Errorf("без ключа: %+v", sug)
	}
	sr, _ := (&Service{}).Search(context.Background(), "plum")
	if sr.Configured || len(sr.Results) != 0 || sr.Notice == "" {
		t.Errorf("поиск без ключа: %+v", sr)
	}
	for _, c := range []struct {
		err    error
		notice string
	}{{ErrRateLimited, "лимит"}, {ErrUnavailable, "недоступен"}} {
		src := plumSource()
		src.err = c.err
		sug, err := (&Service{Source: src}).Suggest(context.Background(), "plum", 0, today, nil)
		if err != nil || sug.Source != "none" || !strings.Contains(sug.Notice, c.notice) || len(sug.Events) != 0 {
			t.Errorf("%v: %+v %v", c.err, sug, err)
		}
	}
}

func TestCachingSavesQuota(t *testing.T) {
	d := testutil.DB(t)
	src := plumSource()
	s := &Service{DB: d, Source: src, Now: func() time.Time { return today }}
	for i := 0; i < 3; i++ {
		_, _ = s.Suggest(context.Background(), "plum", 0, today, nil)
		_, _ = s.Search(context.Background(), "plum")
	}
	if src.searchCalls != 1 || src.detailsCalls != 1 {
		t.Errorf("запросов во внешний источник: search=%d details=%d, ожидалось по 1", src.searchCalls, src.detailsCalls)
	}

	_, _ = s.Search(context.Background(), "zzz")
	_, _ = s.Search(context.Background(), "zzz")
	if src.searchCalls != 2 {
		t.Errorf("негативный кэш: %d", src.searchCalls)
	}
	s.Now = func() time.Time { return today.AddDate(0, 0, 4) }
	_, _ = s.Search(context.Background(), "zzz")
	if src.searchCalls != 3 {
		t.Errorf("после TTL негатива: %d", src.searchCalls)
	}

	bad := plumSource()
	bad.err = ErrUnavailable
	_, _ = (&Service{DB: d, Source: bad}).Search(context.Background(), "rose")
	var n int
	_ = d.QueryRow(`SELECT count(*) FROM plant_lookup_cache WHERE query = 'search:rose'`).Scan(&n)
	if n != 0 {
		t.Errorf("сбой попал в кэш")
	}
}

func TestParseBenchmark(t *testing.T) {
	cases := []struct {
		v, unit  any
		min, max int
	}{
		{"5-7", "days", 5, 7}, {"\"5-7\"", "days", 5, 7}, {"7", "days", 7, 7}, {float64(3), "days", 3, 3},
		{"7-5", "days", 5, 7}, {"1-2", "weeks", 0, 0}, {"Upgrade Plans", "days", 0, 0}, {nil, nil, 0, 0},
	}
	for _, c := range cases {
		a, b := parseBenchmark(c.v, c.unit)
		if a != c.min || b != c.max {
			t.Errorf("%v/%v: %d-%d, ожидалось %d-%d", c.v, c.unit, a, b, c.min, c.max)
		}
	}
}

func TestQueries(t *testing.T) {
	type want struct {
		q       string
		partial bool
	}
	cases := map[string]*want{
		"Слива":              {"plum", false},
		"  алоэ   вера ":     {"aloe vera", false},
		"Monstera deliciosa": {"monstera deliciosa", false},
		"слива домашняя":     {"plum", true},
		"Слива Бербанк":      {"plum", true},
		"Неведомая трава":    nil,
		"":                   nil,
	}
	for in, w := range cases {
		got := queries(in)
		switch {
		case w == nil && len(got) != 0:
			t.Errorf("%q: ожидалось пусто, got %v", in, got)
		case w != nil && (len(got) != 1 || got[0].Q != w.q || got[0].Partial != w.partial):
			t.Errorf("%q: %v, ожидалось %+v", in, got, *w)
		}
	}
}

func TestIntervalRule(t *testing.T) {
	for n, want := range map[int][2]string{1: {"FREQ=DAILY", "Каждый день"}, 7: {"FREQ=WEEKLY", "Раз в неделю"},
		14: {"FREQ=WEEKLY;INTERVAL=2", "Раз в 2 недели"}, 3: {"FREQ=DAILY;INTERVAL=3", "Раз в 3 дня"}, 5: {"FREQ=DAILY;INTERVAL=5", "Раз в 5 дней"}} {
		if r, note := intervalRule(n); r != want[0] || note != want[1] {
			t.Errorf("%d: %s / %s", n, r, note)
		}
	}
}

func TestSuggestionJSONHasNoNullArrays(t *testing.T) {
	s := &Service{Source: plumSource()}
	for _, id := range []int{43, 4000, 42} {
		sug, _ := s.Suggest(context.Background(), "x", id, today, nil)
		b, _ := json.Marshal(sug)
		js := string(b)
		for _, bad := range []string{`"sunlight":null`, `"events":null`, `"coverage":null`, `"channels":null`} {
			if strings.Contains(js, bad) {
				t.Errorf("species %d: %s в %s", id, bad, js)
			}
		}
	}
	none, _ := (&Service{}).Suggest(context.Background(), "x", 0, today, nil)
	b, _ := json.Marshal(none)
	if strings.Contains(string(b), "null,") && (strings.Contains(string(b), `"events":null`) || strings.Contains(string(b), `"coverage":null`)) {
		t.Errorf("none: %s", b)
	}
	sr, _ := (&Service{}).Search(context.Background(), "plum")
	if b, _ := json.Marshal(sr); strings.Contains(string(b), `"results":null`) {
		t.Errorf("search: %s", b)
	}
}
