package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env          string
	HTTPAddr     string
	BaseURL      string
	CookieSecure bool
	DatabaseURL  string

	NotifyHour        int
	SchedulerInterval time.Duration
	OutboxInterval    time.Duration
	OutboxMaxAttempts int

	SMTPHost, SMTPUser, SMTPPassword, SMTPFrom string
	SMTPPort                                   int

	TelegramToken, TelegramBotUsername, TelegramWebhookSecret       string
	VKGroupToken, VKGroupID, VKScreenName, VKConfirmation, VKSecret string
	MAXToken, MAXBotUsername, MAXWebhookSecret                      string

	PerenualKey, PerenualBaseURL string

	DevFakeSenders bool

	BackgroundWorkers bool
	InternalSecret    string
	DBMaxConns        int
}

func (c *Config) IsProd() bool { return c.Env == "prod" }

func Load() (*Config, error) {
	_ = godotenv.Load()
	p := &envParser{}
	c := &Config{
		Env:               get("APP_ENV", "dev"),
		HTTPAddr:          get("HTTP_ADDR", ":8080"),
		BaseURL:           strings.TrimRight(get("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		CookieSecure:      p.bool("COOKIE_SECURE", false),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		NotifyHour:        p.int("NOTIFY_HOUR", 9),
		SchedulerInterval: p.duration("SCHEDULER_INTERVAL", time.Minute),
		OutboxInterval:    p.duration("OUTBOX_INTERVAL", 5*time.Second),
		OutboxMaxAttempts: p.int("OUTBOX_MAX_ATTEMPTS", 4),
		SMTPHost:          get("SMTP_HOST", ""),
		SMTPPort:          p.int("SMTP_PORT", 587),
		SMTPUser:          os.Getenv("SMTP_USER"),
		SMTPPassword:      os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:          get("SMTP_FROM", "no-reply@localhost"),

		TelegramToken:         os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramBotUsername:   os.Getenv("TELEGRAM_BOT_USERNAME"),
		TelegramWebhookSecret: os.Getenv("TELEGRAM_WEBHOOK_SECRET"),
		VKGroupToken:          os.Getenv("VK_GROUP_TOKEN"),
		VKGroupID:             os.Getenv("VK_GROUP_ID"),
		VKScreenName:          os.Getenv("VK_COMMUNITY_SCREEN_NAME"),
		VKConfirmation:        os.Getenv("VK_CONFIRMATION_CODE"),
		VKSecret:              os.Getenv("VK_CALLBACK_SECRET"),
		MAXToken:              os.Getenv("MAX_BOT_TOKEN"),
		MAXBotUsername:        os.Getenv("MAX_BOT_USERNAME"),
		MAXWebhookSecret:      os.Getenv("MAX_WEBHOOK_SECRET"),
		PerenualKey:           os.Getenv("PERENUAL_API_KEY"),
		PerenualBaseURL:       os.Getenv("PERENUAL_BASE_URL"),
		DevFakeSenders:        p.bool("DEV_FAKE_SENDERS", false),
		BackgroundWorkers:     p.bool("BACKGROUND_WORKERS", true),
		InternalSecret:        os.Getenv("INTERNAL_SECRET"),
		DBMaxConns:            p.int("DB_MAX_CONNS", 20),
	}
	if port := os.Getenv("PORT"); port != "" && os.Getenv("HTTP_ADDR") == "" {
		c.HTTPAddr = ":" + port
	}
	if err := errors.Join(p.errs...); err != nil {
		return nil, fmt.Errorf("конфигурация: %w", err)
	}
	if c.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL не задан (см. .env.example)")
	}
	if c.NotifyHour < 0 || c.NotifyHour > 23 {
		return nil, fmt.Errorf("NOTIFY_HOUR должен быть 0..23")
	}
	if c.OutboxMaxAttempts < 1 || c.SchedulerInterval <= 0 || c.OutboxInterval <= 0 {
		return nil, fmt.Errorf("OUTBOX_MAX_ATTEMPTS, SCHEDULER_INTERVAL и OUTBOX_INTERVAL должны быть положительными")
	}
	if c.DBMaxConns < 1 {
		return nil, fmt.Errorf("DB_MAX_CONNS должен быть положительным")
	}
	if !c.BackgroundWorkers && len(c.InternalSecret) < 16 {
		return nil, fmt.Errorf("при BACKGROUND_WORKERS=false нужен INTERNAL_SECRET (не короче 16 символов): им защищён POST /internal/run-jobs")
	}
	if c.IsProd() {
		c.DevFakeSenders = false
		if !c.CookieSecure {
			return nil, fmt.Errorf("в APP_ENV=prod нужен COOKIE_SECURE=true")
		}
	}
	return c, nil
}

func get(k, def string) string {
	if v, ok := os.LookupEnv(k); ok && v != "" {
		return v
	}
	return def
}

type envParser struct{ errs []error }

func (p *envParser) parse(k string, parse func(string) error) {
	if v := os.Getenv(k); v != "" {
		if err := parse(v); err != nil {
			p.errs = append(p.errs, fmt.Errorf("%s=%q: %w", k, v, err))
		}
	}
}

func (p *envParser) int(k string, def int) (n int) {
	n = def
	p.parse(k, func(v string) (err error) { n, err = strconv.Atoi(v); return })
	return
}

func (p *envParser) bool(k string, def bool) (b bool) {
	b = def
	p.parse(k, func(v string) (err error) { b, err = strconv.ParseBool(v); return })
	return
}

func (p *envParser) duration(k string, def time.Duration) (d time.Duration) {
	d = def
	p.parse(k, func(v string) (err error) { d, err = time.ParseDuration(v); return })
	return
}
