package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/thogio8/task-forge/internal/apperror"
	"github.com/thogio8/task-forge/internal/model"
)

type mockDeadLetterTaskStore struct {
	GetAllFunc  func(ctx context.Context) ([]model.DeadLetterTask, error)
	GetByIdFunc func(ctx context.Context, id uuid.UUID) (model.DeadLetterTask, error)
	RetryFunc   func(ctx context.Context, id uuid.UUID) (model.Task, error)
}

func (m *mockDeadLetterTaskStore) GetAll(ctx context.Context) ([]model.DeadLetterTask, error) {
	return m.GetAllFunc(ctx)
}

func (m *mockDeadLetterTaskStore) GetById(ctx context.Context, id uuid.UUID) (model.DeadLetterTask, error) {
	return m.GetByIdFunc(ctx, id)
}

func (m *mockDeadLetterTaskStore) Retry(ctx context.Context, id uuid.UUID) (model.Task, error) {
	return m.RetryFunc(ctx, id)
}

func TestGetDeadLetterTasks_Success(t *testing.T) {
	mock := &mockDeadLetterTaskStore{
		GetAllFunc: func(ctx context.Context) ([]model.DeadLetterTask, error) {
			return []model.DeadLetterTask{
				{
					ID:             uuid.New(),
					OriginalTaskID: uuid.New(),
					Payload:        json.RawMessage(`{"type":"echo"}`),
					LastError:      "something went wrong",
					AttemptCount:   3,
					FailedAt:       time.Now(),
					CreatedAt:      time.Now(),
				},
			}, nil
		},
	}

	h := NewDeadLetterTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/dlq", nil)
	recorder := httptest.NewRecorder()

	h.GetDeadLetterTasks(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusOK)
	}
}

func TestGetDeadLetterTasks_Empty(t *testing.T) {
	mock := &mockDeadLetterTaskStore{
		GetAllFunc: func(ctx context.Context) ([]model.DeadLetterTask, error) {
			return []model.DeadLetterTask{}, nil
		},
	}

	h := NewDeadLetterTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/dlq", nil)
	recorder := httptest.NewRecorder()

	h.GetDeadLetterTasks(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusOK)
	}

	if recorder.Body.String() != "[]\n" {
		t.Errorf("got %v, want %v", recorder.Body.String(), "[]\n")
	}
}

func TestGetDeadLetterTasks_RepoError(t *testing.T) {
	mock := &mockDeadLetterTaskStore{
		GetAllFunc: func(ctx context.Context) ([]model.DeadLetterTask, error) {
			return nil, apperror.Internal("internal server error", nil)
		},
	}

	h := NewDeadLetterTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/dlq", nil)
	recorder := httptest.NewRecorder()

	h.GetDeadLetterTasks(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusInternalServerError)
	}
}

func TestGetDeadLetterTask_Success(t *testing.T) {
	mock := &mockDeadLetterTaskStore{
		GetByIdFunc: func(ctx context.Context, id uuid.UUID) (model.DeadLetterTask, error) {
			return model.DeadLetterTask{
				ID:             id,
				OriginalTaskID: uuid.New(),
				Payload:        json.RawMessage(`{"type":"echo"}`),
				LastError:      "something went wrong",
				AttemptCount:   3,
				FailedAt:       time.Now(),
				CreatedAt:      time.Now(),
			}, nil
		},
	}

	h := NewDeadLetterTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/dlq/{id}", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	h.GetDeadLetterTask(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusOK)
	}
}

func TestGetDeadLetterTask_NotFound(t *testing.T) {
	mock := &mockDeadLetterTaskStore{
		GetByIdFunc: func(ctx context.Context, id uuid.UUID) (model.DeadLetterTask, error) {
			return model.DeadLetterTask{}, apperror.NotFound("dead letter task not found", nil)
		},
	}

	h := NewDeadLetterTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/dlq/{id}", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	h.GetDeadLetterTask(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusNotFound)
	}
}

func TestGetDeadLetterTask_RepoError(t *testing.T) {
	mock := &mockDeadLetterTaskStore{
		GetByIdFunc: func(ctx context.Context, id uuid.UUID) (model.DeadLetterTask, error) {
			return model.DeadLetterTask{}, apperror.Internal("internal server error", nil)
		},
	}

	h := NewDeadLetterTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/dlq/{id}", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	h.GetDeadLetterTask(recorder, request)

	if recorder.Code != http.StatusInternalServerError {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusInternalServerError)
	}
}

func TestGetDeadLetterTask_InvalidID(t *testing.T) {
	mock := &mockDeadLetterTaskStore{}

	h := NewDeadLetterTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("GET", "/dlq/{id}", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "pas-un-uuid")

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	h.GetDeadLetterTask(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusBadRequest)
	}
}

func TestRetry_Success(t *testing.T) {
	mock := &mockDeadLetterTaskStore{
		RetryFunc: func(ctx context.Context, id uuid.UUID) (model.Task, error) {
			return model.Task{
				ID:        uuid.New(),
				Status:    model.StatusPending,
				Payload:   json.RawMessage(`{"type":"echo"}`),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}, nil
		},
	}

	h := NewDeadLetterTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("POST", "/dlq/{id}/retry", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	h.Retry(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusCreated)
	}
}

func TestRetry_NotFound(t *testing.T) {
	mock := &mockDeadLetterTaskStore{
		RetryFunc: func(ctx context.Context, id uuid.UUID) (model.Task, error) {
			return model.Task{}, apperror.NotFound("dead letter task not found", nil)
		},
	}

	h := NewDeadLetterTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("POST", "/dlq/{id}/retry", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", uuid.New().String())

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	h.Retry(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusNotFound)
	}
}

func TestRetry_InvalidID(t *testing.T) {
	mock := &mockDeadLetterTaskStore{}

	h := NewDeadLetterTaskHandler(mock, slog.Default())

	request := httptest.NewRequest("POST", "/dlq/{id}/retry", nil)
	recorder := httptest.NewRecorder()

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "pas-un-uuid")

	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, rctx))

	h.Retry(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Errorf("got %v, want %v", recorder.Code, http.StatusBadRequest)
	}
}
