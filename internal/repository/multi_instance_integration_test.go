//go:build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thogio8/task-forge/internal/model"
)

func TestMultiInstance_ClaimWithoutOverlap(t *testing.T) {
	cleanTasks(t)
	cleanInstances(t)

	taskRepo := NewTaskRepository(testPool, testLogger)

	// Create 6 tasks
	for range 6 {
		task := model.Task{
			Status:  model.StatusPending,
			Payload: []byte(`{"type":"echo"}`),
		}
		_, err := taskRepo.Create(context.Background(), &task)
		if err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}

	// Instance A claims 3 tasks
	claimedA, err := taskRepo.ClaimTasks(context.Background(), "instance-A", 3)
	if err != nil {
		t.Fatalf("instance A failed to claim: %v", err)
	}

	if len(claimedA) != 3 {
		t.Fatalf("instance A: expected 3 claimed, got %d", len(claimedA))
	}

	// Instance B claims 3 tasks
	claimedB, err := taskRepo.ClaimTasks(context.Background(), "instance-B", 3)
	if err != nil {
		t.Fatalf("instance B failed to claim: %v", err)
	}

	if len(claimedB) != 3 {
		t.Fatalf("instance B: expected 3 claimed, got %d", len(claimedB))
	}

	// Verify no overlap — no task ID appears in both sets
	idsA := make(map[string]bool)
	for _, task := range claimedA {
		idsA[task.ID.String()] = true
	}

	for _, task := range claimedB {
		if idsA[task.ID.String()] {
			t.Fatalf("task %s was claimed by both instances", task.ID)
		}
	}
}

func TestMultiInstance_DeadInstanceRecovery(t *testing.T) {
	cleanTasks(t)
	cleanInstances(t)

	taskRepo := NewTaskRepository(testPool, testLogger)
	instanceRepo := NewInstanceRepository(testPool, testLogger, 9999)

	// Register two instances
	err := instanceRepo.Register(context.Background(), "instance-A")
	if err != nil {
		t.Fatalf("failed to register instance A: %v", err)
	}

	err = instanceRepo.Register(context.Background(), "instance-B")
	if err != nil {
		t.Fatalf("failed to register instance B: %v", err)
	}

	// Create 3 tasks and simulate instance A claiming them
	for range 3 {
		task := model.Task{
			Status:  model.StatusPending,
			Payload: []byte(`{"type":"echo"}`),
		}
		_, err := taskRepo.Create(context.Background(), &task)
		if err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}

	_, err = taskRepo.ClaimTasks(context.Background(), "instance-A", 3)
	if err != nil {
		t.Fatalf("failed to claim tasks: %v", err)
	}

	// Simulate instance A dying — set heartbeat in the past
	_, err = testPool.Exec(context.Background(),
		"UPDATE instances SET last_heartbeat_at = NOW() - INTERVAL '1 hour' WHERE id = $1", "instance-A")
	if err != nil {
		t.Fatalf("failed to update heartbeat: %v", err)
	}

	// Instance B detects stale instances
	stale, err := instanceRepo.GetStaleInstances(context.Background(), 30*time.Second)
	if err != nil {
		t.Fatalf("failed to get stale instances: %v", err)
	}

	if len(stale) != 1 {
		t.Fatalf("expected 1 stale instance, got %d", len(stale))
	}

	if stale[0].ID != "instance-A" {
		t.Fatalf("expected stale instance 'instance-A', got '%s'", stale[0].ID)
	}

	// Instance B recovers tasks from dead instance A
	recovered, err := taskRepo.RecoverTasksByInstance(context.Background(), "instance-A")
	if err != nil {
		t.Fatalf("failed to recover tasks: %v", err)
	}

	if recovered != 3 {
		t.Fatalf("expected 3 recovered tasks, got %d", recovered)
	}

	// Instance B deregisters dead instance A
	err = instanceRepo.Deregister(context.Background(), "instance-A")
	if err != nil {
		t.Fatalf("failed to deregister instance A: %v", err)
	}

	// Instance B can now claim the recovered tasks
	claimedB, err := taskRepo.ClaimTasks(context.Background(), "instance-B", 10)
	if err != nil {
		t.Fatalf("failed to claim recovered tasks: %v", err)
	}

	if len(claimedB) != 3 {
		t.Fatalf("expected 3 tasks available after recovery, got %d", len(claimedB))
	}
}

func TestMultiInstance_LeaderElection(t *testing.T) {
	cleanInstances(t)

	// Advisory locks are per-connection, so we need two separate pools
	// to simulate two independent instances
	dbURL := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_SSL_MODE"),
	)

	poolA, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("failed to create pool A: %v", err)
	}
	defer poolA.Close()

	poolB, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("failed to create pool B: %v", err)
	}
	defer poolB.Close()

	repoA := NewInstanceRepository(poolA, testLogger, 9999)
	repoB := NewInstanceRepository(poolB, testLogger, 9999)

	// Instance A tries to become leader
	acquiredA, err := repoA.TryAcquireLeader(context.Background())
	if err != nil {
		t.Fatalf("instance A failed to try leader: %v", err)
	}

	if !acquiredA {
		t.Fatal("expected instance A to acquire leadership")
	}

	// Instance B tries to become leader — should fail
	acquiredB, err := repoB.TryAcquireLeader(context.Background())
	if err != nil {
		t.Fatalf("instance B failed to try leader: %v", err)
	}

	if acquiredB {
		t.Fatal("expected instance B to NOT acquire leadership")
	}

	// Instance A releases leadership
	_, err = repoA.ReleaseLeader(context.Background())
	if err != nil {
		t.Fatalf("instance A failed to release leader: %v", err)
	}

	// Now instance B can acquire leadership
	acquiredB, err = repoB.TryAcquireLeader(context.Background())
	if err != nil {
		t.Fatalf("instance B failed to try leader after release: %v", err)
	}

	if !acquiredB {
		t.Fatal("expected instance B to acquire leadership after A released")
	}

	// Cleanup
	_, _ = repoB.ReleaseLeader(context.Background())
}

func TestMultiInstance_StartupRecovery_IgnoresLiveInstances(t *testing.T) {
	cleanTasks(t)
	cleanInstances(t)

	taskRepo := NewTaskRepository(testPool, testLogger)
	instanceRepo := NewInstanceRepository(testPool, testLogger, 9999)

	// Register two instances
	err := instanceRepo.Register(context.Background(), "live-instance")
	if err != nil {
		t.Fatalf("failed to register live instance: %v", err)
	}

	err = instanceRepo.Register(context.Background(), "dead-instance")
	if err != nil {
		t.Fatalf("failed to register dead instance: %v", err)
	}

	// Create 2 tasks claimed by live instance, 2 by dead instance
	for range 2 {
		task := model.Task{
			Status:  model.StatusPending,
			Payload: []byte(`{"type":"echo"}`),
		}
		_, err := taskRepo.Create(context.Background(), &task)
		if err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}
	claimedLive, err := taskRepo.ClaimTasks(context.Background(), "live-instance", 2)
	if err != nil {
		t.Fatalf("failed to claim for live instance: %v", err)
	}

	for range 2 {
		task := model.Task{
			Status:  model.StatusPending,
			Payload: []byte(`{"type":"echo"}`),
		}
		_, err := taskRepo.Create(context.Background(), &task)
		if err != nil {
			t.Fatalf("failed to create task: %v", err)
		}
	}
	_, err = taskRepo.ClaimTasks(context.Background(), "dead-instance", 2)
	if err != nil {
		t.Fatalf("failed to claim for dead instance: %v", err)
	}

	// Simulate dead instance — set heartbeat in the past
	_, err = testPool.Exec(context.Background(),
		"UPDATE instances SET last_heartbeat_at = NOW() - INTERVAL '1 hour' WHERE id = $1", "dead-instance")
	if err != nil {
		t.Fatalf("failed to update heartbeat: %v", err)
	}

	// Startup recovery: only recover tasks from stale instances
	stale, err := instanceRepo.GetStaleInstances(context.Background(), 30*time.Second)
	if err != nil {
		t.Fatalf("failed to get stale instances: %v", err)
	}

	if len(stale) != 1 {
		t.Fatalf("expected 1 stale instance, got %d", len(stale))
	}

	for _, inst := range stale {
		recovered, err := taskRepo.RecoverTasksByInstance(context.Background(), inst.ID)
		if err != nil {
			t.Fatalf("failed to recover tasks for %s: %v", inst.ID, err)
		}

		err = instanceRepo.Deregister(context.Background(), inst.ID)
		if err != nil {
			t.Fatalf("failed to deregister %s: %v", inst.ID, err)
		}

		if recovered != 2 {
			t.Fatalf("expected 2 recovered tasks from dead instance, got %d", recovered)
		}
	}

	// Verify live instance tasks are still running
	for _, claimed := range claimedLive {
		task, err := taskRepo.GetById(context.Background(), claimed.ID)
		if err != nil {
			t.Fatalf("failed to get live task: %v", err)
		}

		if task.Status != model.StatusRunning {
			t.Fatalf("expected live instance task to still be running, got %s", task.Status)
		}

		if *task.LockedBy != "live-instance" {
			t.Fatalf("expected task locked by 'live-instance', got '%s'", *task.LockedBy)
		}
	}
}
