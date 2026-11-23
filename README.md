# Уход за растениями — напоминания (MVP)

Go + PostgreSQL + React, один бинарник. План разработки: [plant-care-dev-plan.md](plant-care-dev-plan.md).

## Быстрый старт (локально)

```bash
cp .env.example .env        # впишите DATABASE_URL и ключи ботов (токены не коммитятся)
make dev-db                 # PostgreSQL :5433 и Mailpit (SMTP :1025, UI :8025)
make run                    # собирает фронтенд и запускает http://localhost:8080
```

* Приложение: http://localhost:8080 · **Документация API (Swagger): http://localhost:8080/api/docs** (открыта)
* Письма сброса пароля локально: http://localhost:8025
* Пока токены ботов пустые и `DEV_FAKE_SENDERS=true`, сообщения «отправляются» в лог и в таблицу `dev_outbox`.
  Подключение канала можно имитировать вебхуком, например:
  `curl -X POST localhost:8080/webhooks/telegram -H 'X-Telegram-Bot-Api-Secret-Token: <TELEGRAM_WEBHOOK_SECRET>' -d '{"message":{"text":"/start КОД","chat":{"id":1}}}'`
* Разработка фронтенда: `cd web && npm run dev` (порт 5173, прокси на :8080).

## Тесты

```bash
make test
```

* `internal/api/parity_test.go` — **паритет «действие ↔ ручка ↔ документация»**: падает, если в домене появилось действие
  без ручки в API (и записи в `internal/api/registry.go`), если ручка не описана в `openapi.yaml` (и наоборот),
  если операция в спеке без `summary`/схемы ответа, либо если защищённая ручка отвечает не 401 без авторизации.
* Интеграционные тесты (планировщик, outbox, auth, plants) используют БД `plantcare_test`; без `TEST_DATABASE_URL` пропускаются.

## Добавляя новое действие

1. Метод в доменном сервисе → 2. ручка в `internal/api` → 3. описание в `internal/api/openapi.yaml` →
4. запись в `Registry` (`internal/api/registry.go`) → 5. `make test`. Фронтенд использует клиент, сгенерированный из спеки (`npm run gen:api`).

## Прод

`docker compose --profile prod up -d --build` (app + postgres + caddy). Задайте `DOMAIN`, `APP_ENV=prod`, `COOKIE_SECURE=true`,
`PUBLIC_BASE_URL`, SMTP и токены ботов в `.env`.
