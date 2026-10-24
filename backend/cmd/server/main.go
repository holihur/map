// Command server 启动 MapServer HTTP 服务。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"mapserver/internal/api"
	"mapserver/internal/config"
	"mapserver/internal/db"
	"mapserver/internal/store"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	cfg := config.Load()

	// 生产环境禁止使用默认 JWT 密钥
	if cfg.IsProduction() && cfg.UsesDefaultSecret() {
		log.Error("生产环境必须设置 JWT_SECRET（当前为默认值）")
		os.Exit(1)
	}
	if cfg.UsesDefaultSecret() {
		log.Warn("JWT_SECRET 仍为默认值，仅适合开发环境")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 连接数据库（带重试，便于 docker compose 启动顺序）。
	pool, err := connectWithRetry(ctx, cfg.DatabaseURL, log)
	if err != nil {
		log.Error("连接数据库失败", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := db.Migrate(ctx, pool); err != nil {
		log.Error("执行数据库迁移失败", "err", err)
		os.Exit(1)
	}
	created, err := db.EnsureAdmin(ctx, pool, cfg.AdminUsername, cfg.AdminPassword)
	if err != nil {
		log.Error("初始化管理员失败", "err", err)
		os.Exit(1)
	}
	if created {
		log.Warn("已创建默认管理员，请尽快修改密码", "username", cfg.AdminUsername)
	} else if cfg.AdminPassword == "admin123" {
		log.Warn("管理员登录仍在使用默认密码，请尽快修改", "username", cfg.AdminUsername)
	}
	log.Info("数据库已就绪")

	handler := api.NewRouter(store.New(pool), cfg, log)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		if cfg.TLSCertFile != "" && cfg.TLSKeyFile != "" {
			log.Info("MapServer 已启动（HTTPS）", "addr", cfg.Addr)
			if err := srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error("HTTPS 服务异常退出", "err", err)
				stop()
			}
			return
		}
		log.Info("MapServer 已启动", "addr", cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("HTTP 服务异常退出", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("正在关闭服务…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("关闭超时", "err", err)
	}
}

func connectWithRetry(ctx context.Context, dsn string, log *slog.Logger) (*pgxpool.Pool, error) {
	var lastErr error
	for attempt := 1; attempt <= 15; attempt++ {
		pool, err := db.Connect(ctx, dsn)
		if err == nil {
			return pool, nil
		}
		lastErr = err
		log.Warn("等待数据库…", "attempt", attempt, "err", err)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return nil, lastErr
}
