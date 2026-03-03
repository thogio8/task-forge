package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/thogio8/task-forge/internal/apperror"
	"github.com/thogio8/task-forge/internal/model"
	"github.com/thogio8/task-forge/internal/repository"
)

type TaskHandler struct {
	repo   *repository.TaskRepository
	logger *slog.Logger
}

func NewTaskHandler(repo *repository.TaskRepository, logger *slog.Logger) *TaskHandler {
	return &TaskHandler{repo: repo, logger: logger}
}

type CreateTaskRequest struct {
	Payload json.RawMessage `json:"payload"`
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

type TaskResponse struct {
	ID        uuid.UUID       `json:"id"`
	Status    string          `json:"status"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
}

func toTaskResponse(task model.Task) TaskResponse {
	return TaskResponse{
		ID:        task.ID,
		Status:    task.Status,
		Payload:   task.Payload,
		CreatedAt: task.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: task.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *TaskHandler) handleError(w http.ResponseWriter, err error) {
	var notFound *apperror.NotFoundError
	var validation *apperror.ValidationError

	switch {
	case errors.As(err, &notFound):
		Error(w, http.StatusNotFound, notFound.Message)
	case errors.As(err, &validation):
		Error(w, http.StatusBadRequest, validation.Message)
	default:
		Error(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var req CreateTaskRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("invalid request body", "error", err)
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Payload == nil {
		Error(w, http.StatusBadRequest, "payload is required")
		return
	}

	task := model.Task{
		Status:  "pending",
		Payload: req.Payload,
	}

	if err := h.repo.Create(r.Context(), &task); err != nil {
		h.handleError(w, err)
		return
	}

	JSON(w, http.StatusCreated, toTaskResponse(task))
}

func (h *TaskHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	tasks, err := h.repo.GetAll(r.Context())
	if err != nil {
		h.handleError(w, err)
		return
	}

	response := make([]TaskResponse, len(tasks))
	for i, task := range tasks {
		response[i] = toTaskResponse(task)
	}

	JSON(w, http.StatusOK, response)
}

func (h *TaskHandler) GetTask(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid task id")
		return
	}

	task, err := h.repo.GetById(r.Context(), id)
	if err != nil {
		h.handleError(w, err)
		return
	}

	JSON(w, http.StatusOK, toTaskResponse(task))
}

func (h *TaskHandler) UpdateTaskStatus(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid task id")
		return
	}

	var req UpdateStatusRequest

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("invalid request body", "error", err)
		Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Status == "" {
		Error(w, http.StatusBadRequest, "status is required")
		return
	}

	if err := h.repo.UpdateStatus(r.Context(), id, req.Status); err != nil {
		h.handleError(w, err)
		return
	}

	JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}
