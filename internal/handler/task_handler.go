package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thogio8/task-forge/internal/model"
	"github.com/thogio8/task-forge/internal/repository"
)

type TaskHandler struct {
	DB *pgxpool.Pool
}

func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id := uuid.New()
	now := time.Now()

	task := model.Task{
		ID:        id,
		Status:    "pending",
		Payload:   "{}",
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := repository.CreateTask(ctx, h.DB, task)

	if err != nil {
		log.Println("Failed to create task : ", err)
		http.Error(w, "Failed to create task", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *TaskHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tasks, err := repository.GetTasks(ctx, h.DB)

	if err != nil {
		log.Println("Failed to fetch tasks : ", err)
		http.Error(w, "Failed to fetch tasks", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(tasks)
}
