package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DirektorBani/datasafe/internal/api"
	"github.com/DirektorBani/datasafe/internal/metadata"
	_ "github.com/DirektorBani/datasafe/internal/metadata/postgres"
	"github.com/DirektorBani/datasafe/internal/observability"
	"github.com/DirektorBani/datasafe/internal/security"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "migrate-boltdb" {
		if err := runMigrateBolt(os.Args[2:]); err != nil {
			slog.Error("migrate-boltdb failed", "err", err)
			os.Exit(1)
		}
		return
	}

	addr := envOr("STORAGE_ADDR", ":9000")
	logLevel := envOr("STORAGE_LOG_LEVEL", "info")
	dataDir := envOr("STORAGE_DATA_DIR", "./data")
	region := envOr("STORAGE_REGION", "us-east-1")
	accessKey := envOr("STORAGE_ACCESS_KEY", "datasafe")
	secretKey := envOr("STORAGE_SECRET_KEY", "datasafesecret")
	adminUser := envOr("STORAGE_ADMIN_USER", "admin")
	adminPassword := envOr("STORAGE_ADMIN_PASSWORD", "admin")
	jwtSecret := envOr("STORAGE_JWT_SECRET", "datasafe-jwt-secret")
	sseKey := envOr("STORAGE_SSE_MASTER_KEY", "")

	logger := observability.NewJSONLogger(observability.LoggerOptions{Level: logLevel})
	slog.SetDefault(logger)
	security.ValidateStartupSecrets(logger)

	metaCfg := metadata.ConfigFromEnv(dataDir)
	logger.Info("metadata backend", "backend", metaCfg.Backend)

	srv, err := api.NewServer(api.Config{
		DataDir:       dataDir,
		Region:        region,
		AccessKey:     accessKey,
		SecretKey:     secretKey,
		AdminUser:     adminUser,
		AdminPassword: adminPassword,
		JWTSecret:     jwtSecret,
		Metadata:      metaCfg,
		SSEMasterKey:  sseKey,
	})
	if err != nil {
		logger.Error("failed to start server", "err", err)
		os.Exit(1)
	}
	defer srv.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	srv.StartBackground(ctx)

	handler := withRequestLogging(logger, observability.MetricsMiddleware(srv.Handler()))

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		logger.Info("http server listening",
			"addr", addr,
			"data_dir", dataDir,
			"region", region,
			"admin_user", adminUser,
			"metadata_backend", metaCfg.Backend,
		)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "err", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	logger.Info("shutting down")
	_ = httpSrv.Shutdown(shutdownCtx)
}

func withRequestLogging(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.status,
			"duration_ms", time.Since(start).Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(statusCode int) {
	w.status = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
