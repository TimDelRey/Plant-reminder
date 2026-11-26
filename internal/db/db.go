package db

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"plantcare/migrations"
)

func Open(ctx context.Context, url string, maxConns int) (*sql.DB, error) {
	d, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(maxConns)
	d.SetMaxIdleConns(min(maxConns, 5))

	d.SetConnMaxIdleTime(time.Minute)
	d.SetConnMaxLifetime(10 * time.Minute)
	if err := d.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("подключение к БД: %w", err)
	}
	return d, nil
}

func Migrate(ctx context.Context, d *sql.DB) error {
	sqlFS, err := fs.Sub(migrations.FS, "sql")
	if err != nil {
		return err
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return err
	}
	p, err := goose.NewProvider(goose.DialectPostgres, d, sqlFS, goose.WithSessionLocker(locker))
	if err != nil {
		return err
	}
	_, err = p.Up(ctx)
	return err
}

func SeedCities(ctx context.Context, d *sql.DB) error {
	f, err := migrations.FS.Open("data/cities_ru.csv")
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil {
		return err
	}
	records, err := r.ReadAll()
	if err != nil {
		return err
	}
	unique := map[string]struct{}{}
	for _, rec := range records {
		lat, _ := strconv.ParseFloat(rec[4], 64)
		lon, _ := strconv.ParseFloat(rec[5], 64)
		unique[fmt.Sprintf("%s|%s|%.0f|%.0f", strings.ToLower(rec[0]), rec[1], lat*1000, lon*1000)] = struct{}{}
	}
	var have int
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM cities`).Scan(&have); err != nil {
		return err
	}
	if have >= len(unique) {
		return nil
	}

	var names, regions, zones []string
	var pops []int32
	var lats, lons []float64
	seen := map[string]struct{}{}
	for _, rec := range records {
		pop, _ := strconv.Atoi(rec[3])
		lat, _ := strconv.ParseFloat(rec[4], 64)
		lon, _ := strconv.ParseFloat(rec[5], 64)
		key := fmt.Sprintf("%s|%s|%.0f|%.0f", strings.ToLower(rec[0]), rec[1], lat*1000, lon*1000)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		names, regions, zones = append(names, rec[0]), append(regions, rec[1]), append(zones, rec[2])
		pops, lats, lons = append(pops, int32(pop)), append(lats, lat), append(lons, lon)
	}
	_, err = d.ExecContext(ctx, `INSERT INTO cities (name, region, timezone, population, lat, lon)
		SELECT t.name, t.region, t.tz, t.pop, t.lat, t.lon
		FROM unnest($1::text[], $2::text[], $3::text[], $4::int[], $5::float8[], $6::float8[]) WITH ORDINALITY
			AS t(name, region, tz, pop, lat, lon, ord)
		WHERE NOT EXISTS (SELECT 1 FROM cities c WHERE lower(c.name) = lower(t.name) AND c.region = t.region
			AND abs(c.lat - t.lat) < 0.001 AND abs(c.lon - t.lon) < 0.001)
		ORDER BY t.ord`, names, regions, zones, pops, lats, lons)
	return err
}
