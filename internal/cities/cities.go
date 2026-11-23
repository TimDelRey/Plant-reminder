// Package cities — справочник городов РФ с часовыми поясами (данные GeoNames, CC BY 4.0).
package cities

import (
	"context"
	"database/sql"
	"strings"

	"plantcare/internal/apperr"
)

type City struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Region   string `json:"region"`
	Timezone string `json:"timezone"`
}

type Service struct {
	DB *sql.DB
}

var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// Search ищет города по началу названия; без запроса возвращает самые крупные.
func (s *Service) Search(ctx context.Context, q string, limit int) ([]City, error) {
	if limit <= 0 || limit > 50 {
		limit = 15
	}
	q = strings.TrimSpace(q)
	var (
		rows *sql.Rows
		err  error
	)
	if q == "" {
		rows, err = s.DB.QueryContext(ctx, `SELECT id, name, region, timezone FROM cities ORDER BY population DESC, id LIMIT $1`, limit)
	} else {
		rows, err = s.DB.QueryContext(ctx,
			`SELECT id, name, region, timezone FROM cities WHERE lower(name) LIKE lower($1) || '%' ORDER BY population DESC, id LIMIT $2`,
			likeEscaper.Replace(q), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []City{}
	for rows.Next() {
		var c City
		if err := rows.Scan(&c.ID, &c.Name, &c.Region, &c.Timezone); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type Querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ByID возвращает город или apperr 422 для несуществующего city_id.
func ByID(ctx context.Context, q Querier, id int64) (City, error) {
	var c City
	err := q.QueryRowContext(ctx, `SELECT id, name, region, timezone FROM cities WHERE id = $1`, id).
		Scan(&c.ID, &c.Name, &c.Region, &c.Timezone)
	if err == sql.ErrNoRows {
		return c, apperr.Validation(map[string]string{"city_id": "Выберите город из списка"})
	}
	return c, err
}
