package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/ysqss/shortlink/internal/analytics"
	"github.com/ysqss/shortlink/internal/api"
	"github.com/ysqss/shortlink/internal/cache"
	"github.com/ysqss/shortlink/internal/config"
	"github.com/ysqss/shortlink/internal/core"
	"github.com/ysqss/shortlink/internal/metrics"
	"github.com/ysqss/shortlink/internal/model"
	"github.com/ysqss/shortlink/internal/store"
)

var (
	configPath = flag.String("config", "config.yaml", "path to config file")
)

func main() {
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfgMgr, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	cfg := cfgMgr.Get()

	if err := os.MkdirAll("data", 0755); err != nil {
		slog.Error("failed to create data directory", "error", err)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite", cfg.Database.Path)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	st, err := store.New(db)
	if err != nil {
		slog.Error("failed to init store", "error", err)
		os.Exit(1)
	}

	cacheSize := cfg.Short.CacheSize
	if cacheSize <= 0 {
		cacheSize = 10000
	}
	linkCache := cache.NewShardedLRU[string, *model.ShortLink](cacheSize)

	svc := core.NewService(st, linkCache)

	an := analytics.New(st, 4096)

	metrics.SetAnalyticsGauges(
		func() float64 { return float64(an.Dropped()) },
		func() float64 { return float64(an.Inflight()) },
	)

	server := api.NewServer(svc, st, an, cfgMgr)
	handler := api.ApplyMiddleware(server.Handler(), cfgMgr)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/", handler)

	httpServer := &http.Server{
		Addr:         cfg.Server.Listen,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		slog.Info("shortlink server starting", "addr", cfg.Server.Listen)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	if cfg.Server.DebugPort != "" {
		go func() {
			pprofMux := http.NewServeMux()
			pprofMux.Handle("/debug/pprof/", http.DefaultServeMux)
			pprofServer := &http.Server{
				Addr:    cfg.Server.DebugPort,
				Handler: pprofMux,
			}
			slog.Info("pprof server starting", "addr", cfg.Server.DebugPort)
			if err := pprofServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("pprof server error", "error", err)
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigCh {
		switch sig {
		case syscall.SIGHUP:
			slog.Info("received SIGHUP, reloading config")
			if err := cfgMgr.Reload(); err != nil {
				slog.Error("failed to reload config", "error", err)
			} else {
				slog.Info("config reloaded successfully")
			}
		case syscall.SIGINT, syscall.SIGTERM:
			slog.Info("shutting down", "signal", sig.String())

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			httpServer.SetKeepAlivesEnabled(false)
			if err := httpServer.Shutdown(ctx); err != nil {
				slog.Error("http server shutdown error", "error", err)
			}

			an.Shutdown(10 * time.Second)

			slog.Info("shortlink stopped")
			return
		}
	}
}

func printBanner(cfg *config.Config) {
	fmt.Printf(`
   ____  _               _   _      _      _
  / ___|| |__   ___  _ _| | | |   _(_)_ __| | __
  \___ \| '_ \ / _ \| '__| |_| | | | | '__| |/ /
   ___) | | | | (_) | |  |  _  | |_| | |  |   <
  |____/|_| |_|\___/|_|  |_| |_|\__,_|_|  |_|\_\

  %s
`, cfg.Server.PublicURL)
}