package main

import (
	"context"
	"log"
	"net"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"devsec-contest01/internal/config"
	"devsec-contest01/internal/server"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("failed to init logger: %v", err)
	}
	defer logger.Sync()

	cfg, err := config.Load("config.toml")
	if err != nil {
		logger.Fatal("failed to load config", zap.Error(err))
	}
	if cfg.AdminToken == "dev-admin-token" {
		logger.Warn("ADMIN_TOKEN is still the dev placeholder — override it before the real event")
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Fatal("failed to create pg pool", zap.Error(err))
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		logger.Fatal("failed to ping database", zap.Error(err))
	}

	r, err := server.New(ctx, cfg, pool)
	if err != nil {
		logger.Fatal("failed to initialize server", zap.Error(err))
	}

	logger.Info("starting server", zap.String("port", cfg.Port))
	if err := r.Run(net.JoinHostPort(cfg.ListenHost, cfg.Port)); err != nil {
		logger.Fatal("server exited", zap.Error(err))
	}
}
