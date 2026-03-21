package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/thogio8/task-forge/internal/apperror"
	"github.com/thogio8/task-forge/internal/model"
)

type DeadLetterTaskStore interface {
	GetAll(ctx context.Context) ([]model.DeadLetterTask, error)
	GetById(ctx context.Context, id uuid.UUID) (model.DeadLetterTask, error)
	Retry(ctx context.Context, id uuid.UUID) (model.Task, error)
}

type DeadLetterTaskHandler struct {
	deadLetterTaskStore DeadLetterTaskStore
	logger              *slog.Logger
}

func NewDeadLetterTaskHandler(deadLetterTaskStore DeadLetterTaskStore, logger *slog.Logger) *DeadLetterTaskHandler {
	return &DeadLetterTaskHandler{deadLetterTaskStore: deadLetterTaskStore, logger: logger}
}

type DeadLetterTaskResponse struct {
	ID             uuid.UUID       `json:"id"`
	OriginalTaskID uuid.UUID       `json:"original_task_id"`
	Payload        json.RawMessage `json:"payload"`
	LastError      string          `json:"last_error"`
	AttemptCount   int             `json:"attempt_count"`
	FailedAt       string          `json:"failed_at"`
	RetriedAt      *string         `json:"retried_at,omitempty"`
	CreatedAt      string          `json:"created_at"`
}

func toDeadLetterTaskResponse(deadLetterTask model.DeadLetterTask) DeadLetterTaskResponse {
	var retriedAt *string

	if deadLetterTask.RetriedAt != nil {
		retriedAt = new(deadLetterTask.RetriedAt.Format("2006-01-02T15:04:05Z"))
	}

	return DeadLetterTaskResponse{
		ID:             deadLetterTask.ID,
		OriginalTaskID: deadLetterTask.OriginalTaskID,
		Payload:        deadLetterTask.Payload,
		LastError:      deadLetterTask.LastError,
		AttemptCount:   deadLetterTask.AttemptCount,
		FailedAt:       deadLetterTask.FailedAt.Format("2006-01-02T15:04:05Z"),
		RetriedAt:      retriedAt,
		CreatedAt:      deadLetterTask.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

func (h *DeadLetterTaskHandler) handleError(w http.ResponseWriter, err error) {
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

func (h *DeadLetterTaskHandler) GetDeadLetterTasks(w http.ResponseWriter, r *http.Request) {
	deadLetterTasks, err := h.deadLetterTaskStore.GetAll(r.Context())
	if err != nil {
		h.handleError(w, err)
		return
	}

	response := make([]DeadLetterTaskResponse, len(deadLetterTasks))
	for i, deadLetterTask := range deadLetterTasks {
		response[i] = toDeadLetterTaskResponse(deadLetterTask)
	}

	JSON(w, http.StatusOK, response)
}

func (h *DeadLetterTaskHandler) GetDeadLetterTask(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid dead letter task id")
		return
	}

	deadLetterTask, err := h.deadLetterTaskStore.GetById(r.Context(), id)
	if err != nil {
		h.handleError(w, err)
		return
	}

	JSON(w, http.StatusOK, toDeadLetterTaskResponse(deadLetterTask))
}

func (h *DeadLetterTaskHandler) Retry(w http.ResponseWriter, r *http.Request) {
	idParam := chi.URLParam(r, "id")

	id, err := uuid.Parse(idParam)
	if err != nil {
		Error(w, http.StatusBadRequest, "invalid dead letter task id")
		return
	}

	task, err := h.deadLetterTaskStore.Retry(r.Context(), id)
	if err != nil {
		h.handleError(w, err)
		return
	}

	JSON(w, http.StatusCreated, toTaskResponse(task))
}
