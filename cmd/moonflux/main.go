package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	appconfig "moonflux/internal/config"
	"moonflux/internal/database"
	"moonflux/internal/httpserver"
	"moonflux/internal/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "moonflux:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "config/config.yml", "path to YAML config")
	flag.Parse()
	args := flag.Args()

	cfg, err := appconfig.Load(*configPath)
	if err != nil {
		return err
	}
	log, err := logger.New(logger.Config{Level: cfg.Logging.Level, Format: cfg.Logging.Format, AddSource: cfg.Logging.AddSource})
	if err != nil {
		return err
	}
	log = log.With("app", cfg.App.Name, "environment", cfg.App.Environment)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := database.Open(connectCtx, cfg.Database, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	if len(args) > 0 && args[0] == "migrate" {
		if len(args) != 2 || args[1] != "up" {
			return fmt.Errorf("usage: moonflux [--config path] migrate up")
		}
		if err := database.Up(ctx, pool, cfg.Migrations.Directory); err != nil {
			return err
		}
		log.Info("database migrations are up to date")
		return nil
	}
	if len(args) > 0 {
		return fmt.Errorf("unknown command %q", args[0])
	}

	server := httpserver.NewServer(cfg, log, pool)
	return httpserver.Serve(ctx, server, cfg.App.ShutdownTimeout, log)
}
