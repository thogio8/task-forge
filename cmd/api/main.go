package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/thogio8/task-forge/internal/config"
	"github.com/thogio8/task-forge/internal/handler"
	"github.com/thogio8/task-forge/internal/model"
	"github.com/thogio8/task-forge/internal/repository"
	"github.com/thogio8/task-forge/internal/worker"
	"github.com/thogio8/task-forge/internal/worker/handlers"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	cfg, err := config.Load()

	if err != nil {
		log.Fatal("Failed to load config : ", err)
	}

	logger := cfg.GetSlogLogger()
	slog.SetDefault(logger)

	db, err := pgxpool.New(context.Background(), cfg.DatabaseURL())
	if err != nil {
		logger.Error("Failed to connect to DB", "error", err)
		os.Exit(1)
	}

	if err := db.Ping(context.Background()); err != nil {
		logger.Error("Cannot reach DB", "error", err)
		os.Exit(1)
	}

	logger.Info("DB Connected")

	taskRepo := repository.NewTaskRepository(db, logger)
	taskHandler := handler.NewTaskHandler(taskRepo, logger)

	recovered, err := taskRepo.RecoverStaleTasks(context.Background())

	if err != nil {
		logger.Error("cannot recover stale tasks", "error", err)
		os.Exit(1)
	}

	if recovered > 0 {
		logger.Info("stale tasks successfully recovered", "count", recovered)
	}

	executor := worker.NewExecutor(taskRepo, cfg.WorkerTaskTimeout, logger)

	executor.Register("echo", handlers.Echo(logger))

	processFunc := func(task model.Task) {
		executor.Execute(context.Background(), task)
	}

	pool := worker.NewPool(cfg.WorkerPoolSize, processFunc, logger)

	pool.Start()

	dispatcher := worker.NewDispatcher(taskRepo, pool.Tasks(), cfg.WorkerPollInterval, cfg.WorkerBatchSize, logger)

	ctx, cancel := context.WithCancel(context.Background())

	go dispatcher.Run(ctx)

	worker.StartStaleCleaner(ctx, taskRepo, cfg.WorkerStaleInterval, cfg.WorkerStaleDuration, logger)

	router := chi.NewRouter()
	router.Use(middleware.Logger)

	router.Get("/health", handler.HealthCheck)
	router.Post("/tasks", taskHandler.CreateTask)
	router.Get("/tasks", taskHandler.GetTasks)
	router.Get("/tasks/{id}", taskHandler.GetTask)
	router.Patch("/tasks/{id}/status", taskHandler.UpdateTaskStatus)
	router.Route("/debug/pprof", func(r chi.Router) {
		r.HandleFunc("/", pprof.Index)
		r.HandleFunc("/cmdline", pprof.Cmdline)
		r.HandleFunc("/profile", pprof.Profile)
		r.HandleFunc("/symbol", pprof.Symbol)
		r.HandleFunc("/trace", pprof.Trace)
		r.HandleFunc("/{profile}", pprof.Index)
	})

	logger.Info("server starting", "port", cfg.HTTPPort)
	httpServer := &http.Server{
		Addr:         ":" + cfg.HTTPPort,
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}

	logger.Info("http server stopped")

	cancel()
	dispatcher.Stop()

	pool.Stop()
	logger.Info("worker pool stopped")

	db.Close()
	logger.Info("server stopped")
}
