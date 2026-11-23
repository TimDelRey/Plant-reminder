package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"plantcare/internal/api"
	"plantcare/internal/auth"
	"plantcare/internal/cities"
	"plantcare/internal/config"
	"plantcare/internal/db"
	"plantcare/internal/mailer"
	"plantcare/internal/plantinfo"
	"plantcare/internal/plants"
	"plantcare/internal/scheduler"
	"plantcare/internal/sender"
	"plantcare/internal/sender/max"
	"plantcare/internal/sender/telegram"
	"plantcare/internal/sender/vk"
	"plantcare/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("сервер остановлен с ошибкой", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := db.Migrate(ctx, d); err != nil {
		return err
	}
	if err := db.SeedCities(ctx, d); err != nil {
		return err
	}

	// ---- sender: адаптеры каналов ----
	dispatcher := sender.NewDispatcher()
	setup := map[sender.Channel]sender.Setup{}
	register := func(ch sender.Channel, token, username string, build func() (sender.Sender, error)) error {
		switch {
		case token != "":
			s, err := build()
			if err != nil {
				return err
			}
			dispatcher.Register(ch, s)
			setup[ch] = sender.Setup{Configured: true, BotUsername: username}
		case cfg.DevFakeSenders:
			slog.Warn("токена нет — используется dev-заглушка отправщика", "channel", ch)
			dispatcher.Register(ch, &sender.DevSender{DB: d, Channel: ch})
			setup[ch] = sender.Setup{Configured: true, BotUsername: username}
		default:
			setup[ch] = sender.Setup{Configured: false}
		}
		return nil
	}
	if err := register(sender.ChannelTelegram, cfg.TelegramToken, cfg.TelegramBotUsername, func() (sender.Sender, error) { return telegram.New(cfg.TelegramToken) }); err != nil {
		return err
	}
	if err := register(sender.ChannelVK, cfg.VKGroupToken, cfg.VKScreenName, func() (sender.Sender, error) { return vk.New(cfg.VKGroupToken), nil }); err != nil {
		return err
	}
	if err := register(sender.ChannelMAX, cfg.MAXToken, cfg.MAXBotUsername, func() (sender.Sender, error) { return max.New(cfg.MAXToken), nil }); err != nil {
		return err
	}

	var mail mailer.Mailer = mailer.Log{}
	if cfg.SMTPHost != "" {
		mail = &mailer.SMTP{Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser, Password: cfg.SMTPPassword, From: cfg.SMTPFrom}
	}

	info := &plantinfo.Service{DB: d}
	if cfg.PerenualKey != "" {
		info.Source = plantinfo.NewClient(cfg.PerenualKey, cfg.PerenualBaseURL)
	} else {
		slog.Warn("PERENUAL_API_KEY не задан — справочник растений недоступен, события добавляются вручную")
	}

	authSvc := &auth.Service{DB: d, Mailer: mail, BaseURL: cfg.BaseURL}
	handler := api.NewRouter(api.Deps{
		Cfg:     cfg,
		Auth:    authSvc,
		Plants:  &plants.Service{DB: d, Info: info},
		Cities:  &cities.Service{DB: d},
		Linking: &sender.Linking{DB: d, Setup: setup, Dispatcher: dispatcher},
		Static:  staticFS(),
	})

	// фоновые воркеры; при остановке ждём их до закрытия БД
	var workers sync.WaitGroup
	defer workers.Wait()
	for _, run := range []func(context.Context){
		(&scheduler.Scheduler{DB: d, NotifyHour: cfg.NotifyHour, Interval: cfg.SchedulerInterval}).Run,
		(&sender.OutboxWorker{DB: d, Dispatcher: dispatcher, Interval: cfg.OutboxInterval, MaxAttempts: cfg.OutboxMaxAttempts}).Run,
	} {
		workers.Add(1)
		go func() { defer workers.Done(); run(ctx) }()
	}

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second}
	errc := make(chan error, 1)
	go func() {
		slog.Info("сервер запущен", "addr", cfg.HTTPAddr, "env", cfg.Env, "docs", cfg.BaseURL+"/api/docs")
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shCtx)
	}
	return nil
}

func staticFS() fs.FS {
	sub, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
