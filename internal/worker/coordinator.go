package worker

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

type LeaderRepository interface {
	TryAcquireLeader(ctx context.Context) (bool, error)
	ReleaseLeader(ctx context.Context) (bool, error)
}

type Coordinator struct {
	leaderRepo       LeaderRepository
	instanceRepo     InstanceMonitorRepository
	taskRepo         TaskRecoveryRepository
	staleRepo        StaleCleanerRepository
	isLeader         atomic.Bool
	leaderCancel     context.CancelFunc
	leaderInterval   time.Duration
	monitorInterval  time.Duration
	heartbeatTimeout time.Duration
	staleInterval    time.Duration
	staleDuration    time.Duration
	logger           *slog.Logger
}

func NewCoordinator(
	leaderRepo LeaderRepository,
	instanceRepo InstanceMonitorRepository,
	taskRepo TaskRecoveryRepository,
	staleRepo StaleCleanerRepository,
	leaderInterval time.Duration,
	monitorInterval time.Duration,
	heartbeatTimeout time.Duration,
	staleInterval time.Duration,
	staleDuration time.Duration,
	logger *slog.Logger,
) *Coordinator {
	return &Coordinator{
		leaderRepo:       leaderRepo,
		instanceRepo:     instanceRepo,
		taskRepo:         taskRepo,
		staleRepo:        staleRepo,
		leaderInterval:   leaderInterval,
		monitorInterval:  monitorInterval,
		heartbeatTimeout: heartbeatTimeout,
		staleInterval:    staleInterval,
		staleDuration:    staleDuration,
		logger:           logger,
	}
}

func (c *Coordinator) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(c.leaderInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				acquired, err := c.leaderRepo.TryAcquireLeader(ctx)
				if err != nil {
					c.logger.ErrorContext(ctx, "failed to acquire leader", "error", err)
					continue
				}

				if acquired && !c.isLeader.Load() {
					c.isLeader.Store(true)
					c.logger.InfoContext(ctx, "acquired leadership")

					leaderCtx, cancel := context.WithCancel(ctx)
					c.leaderCancel = cancel

					StartInstanceMonitor(leaderCtx, c.instanceRepo, c.taskRepo, c.monitorInterval, c.heartbeatTimeout, c.logger)
					StartStaleCleaner(leaderCtx, c.staleRepo, c.staleInterval, c.staleDuration, c.logger)
				}

				if !acquired && c.isLeader.Load() {
					c.isLeader.Store(false)
					c.leaderCancel()
					c.logger.InfoContext(ctx, "lost leadership")
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}
