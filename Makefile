.PHONY: dev-db web build run test cities

# локальная БД и Mailpit (UI почты: http://localhost:8025)
dev-db:
	docker compose --profile dev up -d postgres mailpit

web:
	cd web && npm install && npm run build

build: web
	go build -o bin/server ./cmd/server

run: build
	./bin/server

# тесты; интеграционные используют отдельную БД plantcare_test (создаётся один раз)
test:
	@docker compose exec -T postgres psql -U plantcare -tAc "SELECT 1 FROM pg_database WHERE datname='plantcare_test'" | grep -q 1 || docker compose exec -T postgres psql -U plantcare -c "CREATE DATABASE plantcare_test"
	TEST_DATABASE_URL='postgres://plantcare:plantcare@localhost:5433/plantcare_test?sslmode=disable' go test -p 1 ./...

# пересобрать справочник городов из дампов GeoNames (см. scripts/gen_cities.py)
cities:
	@echo "python3 scripts/gen_cities.py cities15000.txt RU.txt admin1CodesASCII.txt migrations/data/cities_ru.csv"
