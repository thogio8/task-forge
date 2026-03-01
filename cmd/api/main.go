package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/go-chi/chi/v5"
	"github.com/joho/godotenv"
	"github.com/thogio8/task-forge/internal/handler"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Fatal("Error loading .env file : ", err)
	}

	db, err := pgxpool.New(context.Background(), os.Getenv("DB_CONN_STRING"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	if err := db.Ping(context.Background()); err != nil {
		log.Fatal("Cannot reach DB : ", err)
	}

	fmt.Println("DB connected")

	taskHandler := &handler.TaskHandler{DB: db}

	router := chi.NewRouter()
	router.HandleFunc("/health", healthCheck)
	router.Post("/tasks", taskHandler.CreateTask)
	router.Get("/tasks", taskHandler.GetTasks)

	log.Println("Server running on :8080")
	if err := http.ListenAndServe(":8080", router); err != nil {
		log.Fatal(err)
	}
}

func healthCheck(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}
