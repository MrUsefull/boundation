package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/MrUsefull/boundation/internal/server"
)

func main() {
	ctx := context.Background()
	cfg := mustLoadConfig()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{AddSource: true, Level: cfg.LogLevel}))

	server.Serve(ctx, cfg, logger)
}

func mustLoadConfig() config.Config {
	path := "config.yml"
	if envPath := os.Getenv("CONFIG_PATH"); envPath != "" {
		path = envPath
	}
	cfg, err := config.Load(path)
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err.Error()))
	}
	return cfg
}
