package plantinfo

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	foundTTL    = 30 * 24 * time.Hour
	notFoundTTL = 3 * 24 * time.Hour
	attribution = "Данные об уходе: Perenual (perenual.com)"
	notConfig   = "Справочник Perenual не подключён (не задан PERENUAL_API_KEY) — автоматический подбор недоступен, добавьте события вручную."
)

type Service struct {
	DB     *sql.DB
	Source Source
	Now    func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func failNotice(err error) string {
	if errors.Is(err, ErrRateLimited) {
		return "Дневной лимит запросов к Perenual исчерпан — повторите завтра или добавьте события вручную."
	}
	return "Perenual сейчас недоступен — повторите позже или добавьте события вручную."
}

func (s *Service) Search(ctx context.Context, text string) (ReferenceSearch, error) {
	out := ReferenceSearch{Configured: s.Source != nil, Results: []Candidate{}}
	if s.Source == nil {
		out.Notice = notConfig
		return out, nil
	}
	qs := queries(text)
	if len(qs) == 0 {
		out.Notice = fmt.Sprintf("Не знаю, как перевести «%s» для поиска в справочнике. Введите английское или латинское название (например, plum или Prunus domestica).", strings.TrimSpace(text))
		return out, nil
	}
	q := qs[0]
	out.Query = q.Q
	if q.Partial {
		out.QueryNote = fmt.Sprintf("«%s» целиком в справочнике не переводится — искали по слову «%s» (%s). Сорт или уточнение справочник не различает.", strings.TrimSpace(text), q.Word, q.Q)
	}
	cands, err := s.search(ctx, q.Q)
	if err != nil {
		slog.Warn("perenual search", "query", q.Q, "err", err)
		out.Notice = failNotice(err)
		return out, nil
	}
	out.Results = cands
	if len(cands) == 0 {
		out.Notice = fmt.Sprintf("В справочнике нет ничего по запросу «%s».", q.Q)
	}
	return out, nil
}

func (s *Service) Suggest(ctx context.Context, plantName string, speciesID int, today time.Time, channels []string) (Suggestion, error) {
	none := func(notice string) Suggestion {
		return Suggestion{Source: "none", Configured: s.Source != nil, Coverage: []Coverage{}, Notice: notice, Events: []SuggestedEvent{}}
	}
	if s.Source == nil {
		return none(notConfig), nil
	}

	var facts *Facts
	var quality, note string
	if speciesID > 0 {
		f, err := s.details(ctx, speciesID)
		if err != nil {
			return none(failNotice(err)), nil
		}
		facts, quality = f, "chosen"
		note = "Вид выбран вручную из справочника."
	} else {
		sr, err := s.Search(ctx, plantName)
		if err != nil {
			return Suggestion{}, err
		}
		if len(sr.Results) == 0 {
			msg := sr.Notice
			if msg == "" {
				msg = "Справочник ничего не нашёл."
			}
			return none(msg + " Попробуйте найти вариант вручную (поиск ниже) или добавьте события сами."), nil
		}
		pick := sr.Results[0]
		f, err := s.details(ctx, pick.ID)
		if err != nil {
			return none(failNotice(err)), nil
		}
		facts = f
		quality, note = matchQuality(plantName, sr, pick)
	}

	if facts.CommonName == "" && facts.Scientific == "" {
		facts.CommonName = fmt.Sprintf("Вид №%d", facts.SpeciesID)
	}
	events, cov := build(facts, today, channels)
	sug := Suggestion{
		Source: "perenual", Configured: true, Attribution: attribution, Coverage: cov, Events: events,
		Match: &Match{SpeciesID: facts.SpeciesID, CommonName: facts.CommonName, Scientific: facts.Scientific, ImageURL: facts.ImageURL,
			Sunlight: sunlightLabels(facts.Sunlight), CareLevel: facts.CareLevel, Quality: quality, QualityNote: note},
	}
	if len(events) == 0 {
		sug.Notice = "Справочник нашёл вид, но данных для автоматического плана по нему нет — выберите другой вариант ниже или добавьте события вручную."
	}
	return sug, nil
}

func matchQuality(plantName string, sr ReferenceSearch, c Candidate) (string, string) {
	hay := strings.ToLower(c.CommonName + " " + c.Scientific + " " + strings.Join(c.OtherNames, " "))
	if sr.QueryNote == "" && strings.Contains(hay, strings.ToLower(sr.Query)) {
		return "exact", fmt.Sprintf("«%s» найдено в справочнике как «%s».", strings.TrimSpace(plantName), c.CommonName)
	}
	if sr.QueryNote != "" {
		return "approximate", fmt.Sprintf("Точного совпадения для «%s» нет. Показан ближайший вид «%s». %s", strings.TrimSpace(plantName), c.CommonName, sr.QueryNote)
	}
	return "approximate", fmt.Sprintf("Точного совпадения для «%s» нет. Показан ближайший вид «%s» — проверьте, то ли это растение.", strings.TrimSpace(plantName), c.CommonName)
}

func (s *Service) search(ctx context.Context, q string) ([]Candidate, error) {
	key := "search:" + q
	var cands []Candidate
	if s.cacheGet(ctx, key, &cands) {
		return cands, nil
	}
	cands, err := s.Source.Search(ctx, q)
	if err != nil {
		return nil, err
	}
	if cands == nil {
		cands = []Candidate{}
	}
	s.cachePut(ctx, key, len(cands) > 0, cands)
	return cands, nil
}

func (s *Service) details(ctx context.Context, id int) (*Facts, error) {
	key := fmt.Sprintf("species:%d", id)
	var f Facts
	if s.cacheGet(ctx, key, &f) {
		return &f, nil
	}
	fp, err := s.Source.Details(ctx, id)
	if err != nil {
		return nil, err
	}
	s.cachePut(ctx, key, !fp.Restricted, fp)
	return fp, nil
}

func (s *Service) cacheGet(ctx context.Context, key string, out any) bool {
	if s.DB == nil {
		return false
	}
	var found bool
	var raw []byte
	var at time.Time
	if err := s.DB.QueryRowContext(ctx, `SELECT found, facts, fetched_at FROM plant_lookup_cache WHERE query = $1`, key).Scan(&found, &raw, &at); err != nil {
		return false
	}
	ttl := notFoundTTL
	if found {
		ttl = foundTTL
	}
	if s.now().Sub(at) > ttl || raw == nil {
		return false
	}
	return json.Unmarshal(raw, out) == nil
}

func (s *Service) cachePut(ctx context.Context, key string, found bool, v any) {
	if s.DB == nil {
		return
	}
	b, _ := json.Marshal(v)
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO plant_lookup_cache (query, found, facts, fetched_at) VALUES ($1,$2,$3::jsonb,$4)
		ON CONFLICT (query) DO UPDATE SET found = EXCLUDED.found, facts = EXCLUDED.facts, fetched_at = EXCLUDED.fetched_at`,
		key, found, string(b), s.now()); err != nil {
		slog.Warn("plantinfo: кэш не записан", "err", err)
	}
}
