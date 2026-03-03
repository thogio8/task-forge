package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/thogio8/task-forge/internal/config"
	"github.com/thogio8/task-forge/internal/handler"
	"github.com/thogio8/task-forge/internal/repository"
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

	db, err := pgxpool.New(context.Background(), cfg.DatabaseURL())
	if err != nil {
		logger.Error("Failed to connect to DB", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Ping(context.Background()); err != nil {
		logger.Error("Cannot reach DB", "error", err)
		os.Exit(1)
	}

	logger.Info("DB Connected")

	taskRepo := repository.NewTaskRepository(db, logger)
	taskHandler := handler.NewTaskHandler(taskRepo, logger)

	router := chi.NewRouter()
	router.Use(middleware.Logger)

	router.Get("/health", handler.HealthCheck)
	router.Post("/tasks", taskHandler.CreateTask)
	router.Get("/tasks", taskHandler.GetTasks)
	router.Get("/tasks/{id}", taskHandler.GetTask)
	router.Patch("/tasks/{id}/status", taskHandler.UpdateTaskStatus)

	logger.Info("server starting", "port", cfg.HTTPPort)
	if err := http.ListenAndServe(":"+cfg.HTTPPort, router); err != nil {
		logger.Error("http server error", "error", err)
		os.Exit(1)
	}
}
