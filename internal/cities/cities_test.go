package cities

import (
	"context"
	"testing"

	"plantcare/internal/db"
	"plantcare/internal/testutil"
)

func TestCrimeaAndSevastopolAreRussianCities(t *testing.T) {
	d := testutil.DB(t)
	s := &Service{DB: d}
	for _, name := range []string{"Севастополь", "Симферополь", "Керчь", "Ялта"} {
		res, err := s.Search(context.Background(), name, 5)
		if err != nil || len(res) == 0 {
			t.Fatalf("%s: %v %v", name, res, err)
		}
		if res[0].Timezone != "Europe/Moscow" {
			t.Errorf("%s: пояс %s, ожидался Europe/Moscow", name, res[0].Timezone)
		}
	}
}

func TestSeedIsIdempotent(t *testing.T) {
	d := testutil.DB(t)
	count := func() (n int) { _ = d.QueryRow(`SELECT count(*) FROM cities`).Scan(&n); return }
	before := count()
	if err := db.SeedCities(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	if after := count(); after != before {
		t.Errorf("повторный сид изменил число городов: %d → %d", before, after)
	}
}
