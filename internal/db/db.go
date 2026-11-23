// Package db — подключение к PostgreSQL, миграции (goose) и сид справочника городов.
package db

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"io/fs"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"plantcare/migrations"
)

func Open(ctx context.Context, url string) (*sql.DB, error) {
	d, err := sql.Open("pgx", url)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(20)
	d.SetMaxIdleConns(10)
	d.SetConnMaxLifetime(30 * time.Minute)
	if err := d.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("подключение к БД: %w", err)
	}
	return d, nil
}

// Migrate применяет миграции. Advisory-lock в БД гарантирует, что при одновременном старте
// нескольких инстансов миграции выполнит только один, остальные дождутся.
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

// SeedCities добавляет в справочник города из встроенного CSV (GeoNames, CC BY 4.0), которых там ещё нет.
// Идемпотентно: можно вызывать при каждом старте, существующие строки (и их id) не трогаются.
func SeedCities(ctx context.Context, d *sql.DB) error {
	f, err := migrations.FS.Open("data/cities_ru.csv")
	if err != nil {
		return err
	}
	defer f.Close()
	r := csv.NewReader(f)
	if _, err := r.Read(); err != nil { // заголовок
		return err
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO cities (name, region, timezone, population, lat, lon)
		SELECT $1::text, $2::text, $3::text, $4::int, $5::float8, $6::float8
		WHERE NOT EXISTS (SELECT 1 FROM cities WHERE lower(name) = lower($1::text) AND region = $2::text
			AND abs(lat - $5::float8) < 0.001 AND abs(lon - $6::float8) < 0.001)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		pop, _ := strconv.Atoi(rec[3])
		lat, _ := strconv.ParseFloat(rec[4], 64)
		lon, _ := strconv.ParseFloat(rec[5], 64)
		if _, err := stmt.ExecContext(ctx, rec[0], rec[1], rec[2], pop, lat, lon); err != nil {
			return err
		}
	}
	return tx.Commit()
}
