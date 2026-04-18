//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func cleanOutbox(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), "DELETE FROM outbox")
	if err != nil {
		t.Fatalf("failed to clean outbox: %v", err)
	}
}

func TestOutbox_GetUnpublished(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	repo := NewOutboxRepository(testPool, testLogger)

	// Insert 2 unpublished + 1 published
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"aaa"}`)
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"bbb"}`)
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload, published_at) VALUES ('task.created', $1, NOW())", `{"task_id":"ccc"}`)

	events, err := repo.GetUnpublished(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 unpublished events, got %d", len(events))
	}

	// Verify ordering by created_at
	if string(events[0].Payload) != `{"task_id": "aaa"}` {
		t.Errorf("expected first event payload to be aaa, got %s", string(events[0].Payload))
	}

	if string(events[1].Payload) != `{"task_id": "bbb"}` {
		t.Errorf("expected second event payload to be bbb, got %s", string(events[1].Payload))
	}
}

func TestOutbox_GetUnpublished_Empty(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	repo := NewOutboxRepository(testPool, testLogger)

	events, err := repo.GetUnpublished(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}

	if events != nil {
		t.Fatalf("expected nil slice for empty outbox, got %v", events)
	}
}

func TestOutbox_GetUnpublished_RespectsLimit(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	repo := NewOutboxRepository(testPool, testLogger)

	for i := range 5 {
		testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", fmt.Sprintf(`{"task_id":"task-%d"}`, i))
	}

	events, err := repo.GetUnpublished(ctx, 3)
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 3 {
		t.Fatalf("expected 3 events (limit), got %d", len(events))
	}
}

func TestOutbox_MarkPublished(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	repo := NewOutboxRepository(testPool, testLogger)

	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"aaa"}`)
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"bbb"}`)

	events, _ := repo.GetUnpublished(ctx, 10)
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Mark first event as published
	err := repo.MarkPublished(ctx, []int64{events[0].ID})
	if err != nil {
		t.Fatal(err)
	}

	// Only 1 unpublished remaining
	remaining, _ := repo.GetUnpublished(ctx, 10)
	if len(remaining) != 1 {
		t.Fatalf("expected 1 unpublished after marking, got %d", len(remaining))
	}

	// Verify published_at is set on the marked event
	var publishedAt *time.Time
	testPool.QueryRow(ctx, "SELECT published_at FROM outbox WHERE id = $1", events[0].ID).Scan(&publishedAt)
	if publishedAt == nil {
		t.Fatal("expected published_at to be set after MarkPublished")
	}
}

func TestOutbox_MarkPublished_All(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	repo := NewOutboxRepository(testPool, testLogger)

	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"aaa"}`)
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"bbb"}`)
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"ccc"}`)

	events, _ := repo.GetUnpublished(ctx, 10)
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	ids := make([]int64, len(events))
	for i, e := range events {
		ids[i] = e.ID
	}

	err := repo.MarkPublished(ctx, ids)
	if err != nil {
		t.Fatal(err)
	}

	remaining, _ := repo.GetUnpublished(ctx, 10)
	if len(remaining) != 0 {
		t.Fatalf("expected 0 unpublished after marking all, got %d", len(remaining))
	}
}

func TestOutbox_CountUnpublished(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	repo := NewOutboxRepository(testPool, testLogger)

	// Insert 3 unpublished + 2 published
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"aaa"}`)
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"bbb"}`)
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"ccc"}`)
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload, published_at) VALUES ('task.created', $1, NOW())", `{"task_id":"ddd"}`)
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload, published_at) VALUES ('task.created', $1, NOW())", `{"task_id":"eee"}`)

	count, err := repo.CountUnpublished(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if count != 3 {
		t.Fatalf("expected 3 unpublished events, got %d", count)
	}
}

func TestOutbox_CountUnpublished_Empty(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	repo := NewOutboxRepository(testPool, testLogger)

	count, err := repo.CountUnpublished(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatalf("expected 0 unpublished events in empty outbox, got %d", count)
	}
}

func TestOutbox_CountUnpublished_OnlyPublished(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	repo := NewOutboxRepository(testPool, testLogger)

	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload, published_at) VALUES ('task.created', $1, NOW())", `{"task_id":"aaa"}`)
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload, published_at) VALUES ('task.created', $1, NOW())", `{"task_id":"bbb"}`)

	count, err := repo.CountUnpublished(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatalf("expected 0 unpublished (all are published), got %d", count)
	}
}

func TestOutbox_Purge(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	repo := NewOutboxRepository(testPool, testLogger)

	// Insert a published event from 2 days ago
	testPool.Exec(ctx,
		"INSERT INTO outbox (event_type, payload, published_at) VALUES ('task.created', $1, NOW() - INTERVAL '2 days')",
		`{"task_id":"old"}`,
	)
	// Insert a published event from 1 hour ago
	testPool.Exec(ctx,
		"INSERT INTO outbox (event_type, payload, published_at) VALUES ('task.created', $1, NOW() - INTERVAL '1 hour')",
		`{"task_id":"recent"}`,
	)
	// Insert an unpublished event
	testPool.Exec(ctx,
		"INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)",
		`{"task_id":"unpublished"}`,
	)

	// Purge events published more than 24h ago
	err := repo.Purge(ctx, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	// Check: old one purged, recent one kept, unpublished one kept
	var count int
	testPool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox").Scan(&count)
	if count != 2 {
		t.Fatalf("expected 2 remaining events (recent + unpublished), got %d", count)
	}
}

func TestOutbox_PurgeDoesNotDeleteUnpublished(t *testing.T) {
	cleanOutbox(t)
	ctx := context.Background()
	repo := NewOutboxRepository(testPool, testLogger)

	// Insert only unpublished events
	testPool.Exec(ctx, "INSERT INTO outbox (event_type, payload) VALUES ('task.created', $1)", `{"task_id":"pending"}`)

	// Purge with 0 retention — should delete all published events, but there are none
	err := repo.Purge(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Unpublished event should still be there
	var count int
	testPool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox").Scan(&count)
	if count != 1 {
		t.Fatalf("expected 1 event (unpublished preserved), got %d", count)
	}
}
