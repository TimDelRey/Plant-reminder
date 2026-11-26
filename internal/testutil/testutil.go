package testutil

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"plantcare/internal/db"
)

func DB(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL не задан — интеграционный тест пропущен")
	}
	d, err := db.Open(context.Background(), url, 5)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if err := db.SeedCities(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`TRUNCATE users, dev_outbox, plant_lookup_cache RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	return d
}

func CityID(t *testing.T, d *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	if err := d.QueryRow(`SELECT id FROM cities WHERE name = $1 ORDER BY population DESC LIMIT 1`, name).Scan(&id); err != nil {
		t.Fatalf("город %q: %v", name, err)
	}
	return id
}
