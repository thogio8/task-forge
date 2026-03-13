package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/thogio8/task-forge/internal/apperror"
	"github.com/thogio8/task-forge/internal/model"
)

type mockTaskStore struct {
	CreateFunc       func(ctx context.Context, task *model.Task) (bool, error)
	GetAllFunc       func(ctx context.Context) ([]model.Task, error)
	GetByIdFunc      func(ctx context.Context, id uuid.UUID) (model.Task, error)
	UpdateStatusFunc func(ctx context.Context, id uuid.UUID, status string) error
}

func (m *mockTaskStore) Create(ctx context.Context, task *model.Task) (bool, error) {
	return m.CreateFunc(ctx, task)
}

func (m *mockTaskStore) GetAll(ctx context.Context) ([]model.Task, error) {
	return m.GetAllFunc(ctx)
}

func (m *mockTaskStore) GetById(ctx context.Context, id uuid.UUID) (model.Task, error) {
	return m.GetByIdFunc(ctx, id)
}

func (m *mockTaskStore) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return m.UpdateStatusFunc(ctx, id, status)
}

func TestCreateTask_Success(t *testing.T) {
	mock := &mockTaskStore{
		CreateFunc: func(ctx context.Context, task *model.Task) (bool, error) {
			return true, nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("POST", "/tasks", strings.NewReader(`{"payload":{"type": "email"}}`))
	recorder := httptest.NewRecorder()

	taskHandler.CreateTask(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusCreated)
	}
}

func TestCreateTask_BadJSON(t *testing.T) {
	mock := &mockTaskStore{
		CreateFunc: func(ctx context.Context, task *model.Task) (bool, error) {
			return true, nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("POST", "/tasks", strings.NewReader("invalid json"))
	recorder := httptest.NewRecorder()

	taskHandler.CreateTask(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusBadRequest)
	}
}

func TestCreateTask_EmptyPayload(t *testing.T) {
	mock := &mockTaskStore{
		CreateFunc: func(ctx context.Context, task *model.Task) (bool, error) {
			return true, nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("POST", "/tasks", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()

	taskHandler.CreateTask(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusBadRequest)
	}
}

func TestCreateTask_RepoError(t *testing.T) {
	mock := &mockTaskStore{
		CreateFunc: func(ctx context.Context, task *model.Task) (bool, error) {
			return false, apperror.Internal("internal server error", nil)
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("POST", "/tasks", strings.NewReader(`{"payload":{"type": "email"}}`))
	recorder := httptest.NewRecorder()

	taskHandler.CreateTask(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusInternalServerError)
	}
}

func TestCreateTask_WithIdempotencyKey_Created(t *testing.T) {
	mock := &mockTaskStore{
		CreateFunc: func(ctx context.Context, task *model.Task) (bool, error) {
			return true, nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("POST", "/tasks", strings.NewReader(`{"payload":{"type": "email"}, "idempotency_key": "some-key"}`))
	recorder := httptest.NewRecorder()

	taskHandler.CreateTask(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Errorf("got %v, expected %v", recorder.Code, http.StatusCreated)
	}
}

func TestCreateTask_WithIdempotencyKey_AlreadyExists(t *testing.T) {
	mock := &mockTaskStore{
		CreateFunc: func(ctx context.Context, task *model.Task) (bool, error) {
			return false, nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("POST", "/tasks", strings.NewReader(`{"payload":{"type": "email"}, "idempotency_key": "some-key"}`))
	recorder := httptest.NewRecorder()

	taskHandler.CreateTask(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("got %v, expected %v", recorder.Code, http.StatusOK)
	}
}

func TestGetTasks_Success(t *testing.T) {
	mock := &mockTaskStore{
		GetAllFunc: func(ctx context.Context) ([]model.Task, error) {
			return []model.Task{
				{
					ID:        uuid.New(),
					Status:    "pending",
					Payload:   json.RawMessage(`{"payload":{"type": "email"}}`),
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
				{
					ID:        uuid.New(),
					Status:    "running",
					Payload:   json.RawMessage(`{"payload":{"type": "plop"}}`),
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
				},
			}, nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/tasks", nil)
	recorder := httptest.NewRecorder()

	taskHandler.GetTasks(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusOK)
	}
}

func TestGetTasks_Empty(t *testing.T) {
	mock := &mockTaskStore{
		GetAllFunc: func(ctx context.Context) ([]model.Task, error) {
			return []model.Task{}, nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/tasks", nil)
	recorder := httptest.NewRecorder()

	taskHandler.GetTasks(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusOK)
	}

	if recorder.Body.String() != "[]\n" {
		t.Errorf("got %v, want %v", recorder.Body.String(), "[]\n")
	}
}

func TestGetTasks_RepoError(t *testing.T) {
	mock := &mockTaskStore{
		GetAllFunc: func(ctx context.Context) ([]model.Task, error) {
			return nil, apperror.Internal("internal server error", nil)
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/tasks", nil)
	recorder := httptest.NewRecorder()

	taskHandler.GetTasks(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusInternalServerError)
	}
}

func TestGetTask_Success(t *testing.T) {
	mock := &mockTaskStore{
		GetByIdFunc: func(ctx context.Context, id uuid.UUID) (model.Task, error) {
			return model.Task{
				ID:        id,
				Status:    "pending",
				Payload:   json.RawMessage(`{"payload":{"type": "email"}}`),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/tasks/{id}", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.GetTask(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusOK)
	}
}

func TestGetTask_NotFound(t *testing.T) {
	mock := &mockTaskStore{
		GetByIdFunc: func(ctx context.Context, id uuid.UUID) (model.Task, error) {
			return model.Task{}, apperror.NotFound("task not found", nil)
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/tasks/{id}", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.GetTask(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusNotFound)
	}
}

func TestGetTask_InvalidID(t *testing.T) {
	mock := &mockTaskStore{
		GetByIdFunc: func(ctx context.Context, id uuid.UUID) (model.Task, error) {
			return model.Task{}, apperror.Validation("invalid id", nil)
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/tasks/{id}", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "pas-un-uuid")

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.GetTask(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusBadRequest)
	}
}

func TestGetTask_RepoError(t *testing.T) {
	mock := &mockTaskStore{
		GetByIdFunc: func(ctx context.Context, id uuid.UUID) (model.Task, error) {
			return model.Task{}, apperror.Internal("internal server error", nil)
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/tasks/{id}", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.GetTask(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusInternalServerError)
	}
}

func TestUpdateStatus_Success(t *testing.T) {
	mock := &mockTaskStore{
		UpdateStatusFunc: func(ctx context.Context, id uuid.UUID, status string) error {
			return nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("PATCH", "/tasks/{id}/status", strings.NewReader(`{"status": "running"}`))
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.UpdateTaskStatus(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusOK)
	}
}

func TestUpdateStatus_InvalidID(t *testing.T) {
	mock := &mockTaskStore{
		UpdateStatusFunc: func(ctx context.Context, id uuid.UUID, status string) error {
			return apperror.Validation("invalid id", nil)
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("PATCH", "/tasks/{id}/status", strings.NewReader(`{"status": "running"}`))
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "pas-un-uuid")

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.UpdateTaskStatus(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusBadRequest)
	}
}

func TestUpdateStatus_BadJSON(t *testing.T) {
	mock := &mockTaskStore{
		UpdateStatusFunc: func(ctx context.Context, id uuid.UUID, status string) error {
			return nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("PATCH", "/tasks/{id}/status", strings.NewReader("invalid json"))
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.UpdateTaskStatus(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusBadRequest)
	}
}

func TestUpdateStatus_EmptyStatus(t *testing.T) {
	mock := &mockTaskStore{
		UpdateStatusFunc: func(ctx context.Context, id uuid.UUID, status string) error {
			return nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("PATCH", "/tasks/{id}/status", strings.NewReader(`{}`))
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.UpdateTaskStatus(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusBadRequest)
	}
}

func TestUpdateStatus_NotFound(t *testing.T) {
	mock := &mockTaskStore{
		UpdateStatusFunc: func(ctx context.Context, id uuid.UUID, status string) error {
			return apperror.NotFound("task not found", nil)
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("PATCH", "/tasks/{id}/status", strings.NewReader(`{"status": "running"}`))
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.UpdateTaskStatus(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusNotFound)
	}
}

func TestUpdateStatus_RepoError(t *testing.T) {
	mock := &mockTaskStore{
		UpdateStatusFunc: func(ctx context.Context, id uuid.UUID, status string) error {
			return apperror.Internal("internal server error", nil)
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("PATCH", "/tasks/{id}/status", strings.NewReader(`{"status": "running"}`))
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.UpdateTaskStatus(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusInternalServerError)
	}
}

func TestUpdateStatus_InvalidStatus(t *testing.T) {
	mock := &mockTaskStore{
		UpdateStatusFunc: func(ctx context.Context, id uuid.UUID, status string) error {
			return nil
		},
	}

	taskHandler := NewTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("PATCH", "/tasks/{id}/status", strings.NewReader(`{"status": "banane"}`))
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	taskHandler.UpdateTaskStatus(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusBadRequest)
	}
}
