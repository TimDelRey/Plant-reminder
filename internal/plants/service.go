// Package plants — растения и события ухода: CRUD, recurrence, лента ближайших дел.
// Пакет не знает про каналы отправки: события хранят только список имён каналов.
package plants

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"plantcare/internal/apperr"
	"plantcare/internal/plantinfo"
)

type Service struct {
	DB   *sql.DB
	Info *plantinfo.Service // подбор ухода (Perenual); nil — только общий набор
	Now  func() time.Time   // для тестов; по умолчанию time.Now
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

const eventCols = `e.id, e.plant_id, e.title, e.category, e.description, e.recurrence_kind, e.recurrence_rule,
	e.recurrence_note, e.next_due_at, array_to_string(e.channels, ','), e.active`

func scanEvent(row interface{ Scan(...any) error }, extra ...any) (CareEvent, error) {
	var ev CareEvent
	var due time.Time
	var ch string
	dest := append([]any{&ev.ID, &ev.PlantID, &ev.Title, &ev.Category, &ev.Description, &ev.RecurrenceKind, &ev.RecurrenceRule,
		&ev.RecurrenceNote, &due, &ch, &ev.Active}, extra...)
	if err := row.Scan(dest...); err != nil {
		return ev, err
	}
	ev.NextDueAt = due.Format(DateLayout)
	ev.Channels = []string{}
	if ch != "" {
		ev.Channels = strings.Split(ch, ",")
	}
	return ev, nil
}

func (s *Service) ownsPlant(ctx context.Context, userID, plantID int64) error {
	var x int
	err := s.DB.QueryRowContext(ctx, `SELECT 1 FROM plants WHERE id = $1 AND user_id = $2`, plantID, userID).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.NotFound()
	}
	return err
}

func (s *Service) userToday(ctx context.Context, userID int64) (time.Time, error) {
	var tz string
	err := s.DB.QueryRowContext(ctx, `SELECT c.timezone FROM users u JOIN cities c ON c.id = u.city_id WHERE u.id = $1`, userID).Scan(&tz)
	if err != nil {
		return time.Time{}, err
	}
	return LocalToday(s.now(), tz), nil
}

// ---------- растения ----------

const plantSelect = `SELECT p.id, p.name, p.emoji, p.created_at,
	(SELECT count(*) FROM care_events e WHERE e.plant_id = p.id),
	(SELECT min(e.next_due_at) FROM care_events e WHERE e.plant_id = p.id AND e.active)
FROM plants p`

func scanPlant(row interface{ Scan(...any) error }) (Plant, error) {
	var p Plant
	var next sql.NullTime
	if err := row.Scan(&p.ID, &p.Name, &p.Emoji, &p.CreatedAt, &p.EventsCount, &next); err != nil {
		return p, err
	}
	if next.Valid {
		d := next.Time.Format(DateLayout)
		p.NextDueAt = &d
	}
	return p, nil
}

func (s *Service) ListPlants(ctx context.Context, userID int64) ([]Plant, error) {
	rows, err := s.DB.QueryContext(ctx, plantSelect+` WHERE p.user_id = $1 ORDER BY p.created_at, p.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Plant{}
	for rows.Next() {
		p, err := scanPlant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func validatePlantName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 100 {
		return "", apperr.Validation(map[string]string{"name": "Введите название (до 100 символов)"})
	}
	return name, nil
}

func validateEmoji(e string) (string, error) {
	e = strings.TrimSpace(e)
	if utf8.RuneCountInString(e) > 16 {
		return "", apperr.Validation(map[string]string{"emoji": "Слишком длинный значок"})
	}
	return e, nil
}

func (s *Service) CreatePlant(ctx context.Context, userID int64, in PlantInput) (Plant, error) {
	name, err := validatePlantName(in.Name)
	if err != nil {
		return Plant{}, err
	}
	emoji, err := validateEmoji(in.Emoji)
	if err != nil {
		return Plant{}, err
	}
	if emoji == "" {
		emoji = "🌱"
	}
	var id int64
	if err := s.DB.QueryRowContext(ctx, `INSERT INTO plants (user_id, name, emoji) VALUES ($1,$2,$3) RETURNING id`, userID, name, emoji).Scan(&id); err != nil {
		return Plant{}, err
	}
	return scanPlant(s.DB.QueryRowContext(ctx, plantSelect+` WHERE p.id = $1`, id))
}

func (s *Service) GetPlant(ctx context.Context, userID, plantID int64) (PlantDetail, error) {
	p, err := scanPlant(s.DB.QueryRowContext(ctx, plantSelect+` WHERE p.id = $1 AND p.user_id = $2`, plantID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return PlantDetail{}, apperr.NotFound()
	}
	if err != nil {
		return PlantDetail{}, err
	}
	evs, err := s.ListEvents(ctx, userID, plantID)
	return PlantDetail{Plant: p, Events: evs}, err
}

// UpdatePlant меняет переданные поля одним атомарным UPDATE; владелец проверяется в том же запросе.
func (s *Service) UpdatePlant(ctx context.Context, userID, plantID int64, in PlantPatch) (Plant, error) {
	var name, emoji *string // nil — поле не меняется (COALESCE оставит текущее значение)
	if in.Name != nil {
		n, err := validatePlantName(*in.Name)
		if err != nil {
			return Plant{}, err
		}
		name = &n
	}
	if in.Emoji != nil {
		e, err := validateEmoji(*in.Emoji)
		if err != nil {
			return Plant{}, err
		}
		if e == "" {
			e = "🌱"
		}
		emoji = &e
	}
	var id int64
	err := s.DB.QueryRowContext(ctx, `UPDATE plants SET name = COALESCE($3, name), emoji = COALESCE($4, emoji)
		WHERE id = $1 AND user_id = $2 RETURNING id`, plantID, userID, name, emoji).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Plant{}, apperr.NotFound()
	}
	if err != nil {
		return Plant{}, err
	}
	return scanPlant(s.DB.QueryRowContext(ctx, plantSelect+` WHERE p.id = $1`, id))
}

func (s *Service) DeletePlant(ctx context.Context, userID, plantID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM plants WHERE id = $1 AND user_id = $2`, plantID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperr.NotFound()
	}
	return nil
}

// ---------- события ----------

func (s *Service) ListEvents(ctx context.Context, userID, plantID int64) ([]CareEvent, error) {
	if err := s.ownsPlant(ctx, userID, plantID); err != nil {
		return nil, err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+eventCols+` FROM care_events e WHERE e.plant_id = $1 ORDER BY e.next_due_at, e.id`, plantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CareEvent{}
	for rows.Next() {
		ev, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

type eventFields struct {
	title, category, description, kind, note string
	rule                                     *string
	due                                      time.Time
	channels                                 []string
	active                                   bool
	dateInvalid                              bool
}

func validateChannels(ch []string) ([]string, string) {
	seen := map[string]bool{}
	out := []string{}
	for _, c := range ch {
		ok := false
		for _, k := range Channels {
			if c == k {
				ok = true
			}
		}
		if !ok {
			return nil, "Неизвестный канал: " + c
		}
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out, ""
}

func (f *eventFields) validate() error {
	fields := map[string]string{}
	if f.dateInvalid {
		fields["next_due_at"] = "Укажите дату в формате ГГГГ-ММ-ДД"
	}
	f.title = strings.TrimSpace(f.title)
	if f.title == "" || utf8.RuneCountInString(f.title) > 200 {
		fields["title"] = "Введите название (до 200 символов)"
	}
	okCat := false
	for _, c := range Categories {
		if c == f.category {
			okCat = true
		}
	}
	if !okCat {
		fields["category"] = "Выберите категорию из списка"
	}
	if utf8.RuneCountInString(f.description) > 2000 {
		fields["description"] = "Максимум 2000 символов"
	}
	if utf8.RuneCountInString(f.note) > 200 {
		fields["recurrence_note"] = "Максимум 200 символов"
	}
	switch f.kind {
	case "rrule":
		if f.rule == nil || strings.TrimSpace(*f.rule) == "" {
			fields["recurrence_rule"] = "Укажите правило повторения"
		} else if _, err := ParseRule(*f.rule, f.due); err != nil {
			fields["recurrence_rule"] = "Некорректное правило: " + err.Error()
		} else {
			r := strings.TrimPrefix(strings.TrimSpace(*f.rule), "RRULE:")
			f.rule = &r
		}
	case "manual":
		f.rule = nil
	default:
		fields["recurrence_kind"] = "Допустимо: rrule или manual"
	}
	chs, msg := validateChannels(f.channels)
	if msg != "" {
		fields["channels"] = msg
	}
	f.channels = chs
	if len(fields) > 0 {
		return apperr.Validation(fields)
	}
	return nil
}

func parseDate(v string) (time.Time, bool) {
	t, err := time.Parse(DateLayout, v)
	return t, err == nil
}

func (s *Service) CreateEvent(ctx context.Context, userID, plantID int64, in EventInput) (CareEvent, error) {
	if err := s.ownsPlant(ctx, userID, plantID); err != nil {
		return CareEvent{}, err
	}
	f := eventFields{title: in.Title, category: in.Category, description: in.Description, kind: in.RecurrenceKind,
		rule: in.RecurrenceRule, note: strings.TrimSpace(in.RecurrenceNote), channels: in.Channels, active: true}
	if in.Active != nil {
		f.active = *in.Active
	}
	due, ok := parseDate(in.NextDueAt)
	f.due, f.dateInvalid = due, !ok
	if err := f.validate(); err != nil {
		return CareEvent{}, err
	}
	return s.insertEvent(ctx, plantID, f)
}

func (s *Service) insertEvent(ctx context.Context, plantID int64, f eventFields) (CareEvent, error) {
	var id int64
	err := s.DB.QueryRowContext(ctx, `INSERT INTO care_events
		(plant_id, title, category, description, recurrence_kind, recurrence_rule, recurrence_note, next_due_at, channels, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,string_to_array($9, ','),$10) RETURNING id`,
		plantID, f.title, f.category, strings.TrimSpace(f.description), f.kind, f.rule, f.note, f.due, strings.Join(f.channels, ","), f.active).Scan(&id)
	if err != nil {
		return CareEvent{}, err
	}
	return s.eventByID(ctx, id)
}

func (s *Service) eventByID(ctx context.Context, id int64) (CareEvent, error) {
	return scanEvent(s.DB.QueryRowContext(ctx, `SELECT `+eventCols+` FROM care_events e WHERE e.id = $1`, id))
}

func (s *Service) ownedEvent(ctx context.Context, userID, eventID int64) (CareEvent, error) {
	ev, err := scanEvent(s.DB.QueryRowContext(ctx, `SELECT `+eventCols+` FROM care_events e
		JOIN plants p ON p.id = e.plant_id WHERE e.id = $1 AND p.user_id = $2`, eventID, userID))
	if errors.Is(err, sql.ErrNoRows) {
		return ev, apperr.NotFound()
	}
	return ev, err
}

func (s *Service) UpdateEvent(ctx context.Context, userID, eventID int64, in EventPatch) (CareEvent, error) {
	cur, err := s.ownedEvent(ctx, userID, eventID)
	if err != nil {
		return CareEvent{}, err
	}
	due, _ := parseDate(cur.NextDueAt)
	f := eventFields{title: cur.Title, category: cur.Category, description: cur.Description, kind: cur.RecurrenceKind,
		rule: cur.RecurrenceRule, note: cur.RecurrenceNote, due: due, channels: cur.Channels, active: cur.Active}
	if in.Title != nil {
		f.title = *in.Title
	}
	if in.Category != nil {
		f.category = *in.Category
	}
	if in.Description != nil {
		f.description = *in.Description
	}
	if in.RecurrenceKind != nil {
		f.kind = *in.RecurrenceKind
	}
	if in.RecurrenceRule != nil {
		f.rule = in.RecurrenceRule
	}
	if in.RecurrenceNote != nil {
		f.note = strings.TrimSpace(*in.RecurrenceNote)
	}
	if in.NextDueAt != nil {
		d, ok := parseDate(*in.NextDueAt)
		f.due, f.dateInvalid = d, !ok
	}
	if in.Channels != nil {
		f.channels = in.Channels
	}
	if in.Active != nil {
		f.active = *in.Active
	}
	if err := f.validate(); err != nil {
		return CareEvent{}, err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE care_events SET title=$1, category=$2, description=$3, recurrence_kind=$4, recurrence_rule=$5,
		recurrence_note=$6, next_due_at=$7, channels=string_to_array($8, ','), active=$9 WHERE id=$10`,
		f.title, f.category, strings.TrimSpace(f.description), f.kind, f.rule, f.note, f.due, strings.Join(f.channels, ","), f.active, eventID)
	if err != nil {
		return CareEvent{}, err
	}
	return s.eventByID(ctx, eventID)
}

func (s *Service) DeleteEvent(ctx context.Context, userID, eventID int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM care_events e USING plants p WHERE e.id = $1 AND p.id = e.plant_id AND p.user_id = $2`, eventID, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apperr.NotFound()
	}
	return nil
}

// CompleteEvent — «отметить выполненным»: пересчитывает next_due_at.
// rrule: ближайшая дата строго после сегодняшней (по поясу пользователя); manual: та же дата через год.
func (s *Service) CompleteEvent(ctx context.Context, userID, eventID int64) (CareEvent, error) {
	ev, err := s.ownedEvent(ctx, userID, eventID)
	if err != nil {
		return CareEvent{}, err
	}
	today, err := s.userToday(ctx, userID)
	if err != nil {
		return CareEvent{}, err
	}
	due, _ := parseDate(ev.NextDueAt)
	if ev.RecurrenceKind == "rrule" && ev.RecurrenceRule != nil {
		base := due
		if today.After(base) {
			base = today
		}
		next, ok, err := NextAfter(*ev.RecurrenceRule, due, base)
		if err != nil {
			return CareEvent{}, apperr.Validation(map[string]string{"recurrence_rule": err.Error()})
		}
		if !ok {
			_, err = s.DB.ExecContext(ctx, `UPDATE care_events SET active = false WHERE id = $1`, eventID)
		} else {
			_, err = s.DB.ExecContext(ctx, `UPDATE care_events SET next_due_at = $1 WHERE id = $2`, next, eventID)
		}
		if err != nil {
			return CareEvent{}, err
		}
	} else {
		_, err = s.DB.ExecContext(ctx, `UPDATE care_events SET next_due_at = $1 WHERE id = $2`, due.AddDate(1, 0, 0), eventID)
		if err != nil {
			return CareEvent{}, err
		}
	}
	return s.eventByID(ctx, eventID)
}

// ListUpcoming — лента ближайших активных дел по всем растениям пользователя.
func (s *Service) ListUpcoming(ctx context.Context, userID int64, limit int) ([]UpcomingEvent, error) {
	if limit <= 0 || limit > 50 {
		limit = 6
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+eventCols+`, p.id, p.name, p.emoji FROM care_events e
		JOIN plants p ON p.id = e.plant_id WHERE p.user_id = $1 AND e.active
		ORDER BY e.next_due_at, e.id LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UpcomingEvent{}
	for rows.Next() {
		var u UpcomingEvent
		ev, err := scanEvent(rows, &u.Plant.ID, &u.Plant.Name, &u.Plant.Emoji)
		if err != nil {
			return nil, err
		}
		u.Event = ev
		out = append(out, u)
	}
	return out, rows.Err()
}

// connectedChannels — подключённые каналы пользователя (по умолчанию для предлагаемых событий).
func (s *Service) connectedChannels(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT channel FROM user_channels WHERE user_id = $1 AND status = 'connected' ORDER BY channel`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// SuggestCare подбирает рекомендации по уходу за растением по справочнику Perenual (ничего не сохраняет).
// speciesID>0 — взять данные конкретного вида из результатов поиска; иначе вид подбирается по названию растения.
func (s *Service) SuggestCare(ctx context.Context, userID, plantID int64, speciesID int) (plantinfo.Suggestion, error) {
	var name string
	err := s.DB.QueryRowContext(ctx, `SELECT name FROM plants WHERE id = $1 AND user_id = $2`, plantID, userID).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return plantinfo.Suggestion{}, apperr.NotFound()
	}
	if err != nil {
		return plantinfo.Suggestion{}, err
	}
	today, err := s.userToday(ctx, userID)
	if err != nil {
		return plantinfo.Suggestion{}, err
	}
	channels, err := s.connectedChannels(ctx, userID)
	if err != nil {
		return plantinfo.Suggestion{}, err
	}
	return s.info().Suggest(ctx, name, speciesID, today, channels)
}

// SearchReference показывает, какие растения и сорта есть в справочнике по запросу (без привязки к растению).
func (s *Service) SearchReference(ctx context.Context, userID int64, q string) (plantinfo.ReferenceSearch, error) {
	if len(strings.TrimSpace(q)) < 2 {
		return plantinfo.ReferenceSearch{}, apperr.Validation(map[string]string{"q": "Введите не меньше двух символов"})
	}
	return s.info().Search(ctx, q)
}

func (s *Service) info() *plantinfo.Service {
	if s.Info == nil {
		return &plantinfo.Service{} // справочник не подключён
	}
	return s.Info
}

// ImportCare сохраняет выбранные пользователем события (например, из предпросмотра SuggestCare).
// Все или ни одного. Событий «по умолчанию» нет: пустой список — ошибка.
func (s *Service) ImportCare(ctx context.Context, userID, plantID int64, events []EventInput) ([]CareEvent, error) {
	if err := s.ownsPlant(ctx, userID, plantID); err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, apperr.Validation(map[string]string{"events": "Выберите хотя бы одно событие"})
	}
	if len(events) > 50 {
		return nil, apperr.Validation(map[string]string{"events": "Не больше 50 событий за раз"})
	}
	// сначала валидируем всё, чтобы не сохранить набор наполовину
	prepared := make([]eventFields, 0, len(events))
	for i, in := range events {
		f := eventFields{title: in.Title, category: in.Category, description: in.Description, kind: in.RecurrenceKind,
			rule: in.RecurrenceRule, note: strings.TrimSpace(in.RecurrenceNote), channels: in.Channels, active: true}
		if in.Active != nil {
			f.active = *in.Active
		}
		due, ok := parseDate(in.NextDueAt)
		f.due, f.dateInvalid = due, !ok
		if err := f.validate(); err != nil {
			if ae, ok := err.(*apperr.Error); ok {
				fields := map[string]string{}
				for k, v := range ae.Fields {
					fields[fmt.Sprintf("events[%d].%s", i, k)] = v
				}
				return nil, apperr.Validation(fields)
			}
			return nil, err
		}
		prepared = append(prepared, f)
	}
	out := make([]CareEvent, 0, len(prepared))
	for _, f := range prepared {
		ev, err := s.insertEvent(ctx, plantID, f)
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}
