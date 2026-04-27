package worker

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/thogio8/task-forge/internal/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
)

type Pool struct {
	workerCount   int
	processFunc   func(context.Context, model.Task)
	tasks         chan model.TaskEnvelope
	wg            sync.WaitGroup
	logger        *slog.Logger
	processed     atomic.Int64
	active        atomic.Int64
	activeWorkers metric.Int64UpDownCounter
}

func NewPool(workerCount int, processFunc func(context.Context, model.Task), logger *slog.Logger) *Pool {
	bufferedChannel := make(chan model.TaskEnvelope, workerCount*2)

	meter := otel.Meter("taskforge.pool")

	activeWorkers, _ := meter.Int64UpDownCounter(
		"pool.active_workers",
		metric.WithDescription("Number of active workers."),
	)

	_, _ = meter.Int64ObservableGauge(
		"pool.queue_depth",
		metric.WithDescription("Number of tasks waiting in the pool channel."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(int64(len(bufferedChannel)))
			return nil
		}),
	)

	return &Pool{
		workerCount:   workerCount,
		processFunc:   processFunc,
		tasks:         bufferedChannel,
		logger:        logger,
		activeWorkers: activeWorkers,
	}
}

func (p *Pool) Start() {
	p.wg.Add(p.workerCount)

	for i := range p.workerCount {
		go func(_ int) {
			defer p.wg.Done()
			for envelope := range p.tasks {
				p.active.Add(1)
				p.activeWorkers.Add(envelope.Ctx, 1)
				p.processFunc(envelope.Ctx, envelope.Task)
				p.activeWorkers.Add(envelope.Ctx, -1)
				p.active.Add(-1)
				p.processed.Add(1)
			}
		}(i)
	}
}

func (p *Pool) Submit(ctx context.Context, task model.Task) {
	p.tasks <- model.TaskEnvelope{Task: task, Ctx: ctx}
}

func (p *Pool) Stop() {
	close(p.tasks)
	p.wg.Wait()
}

func (p *Pool) Tasks() chan model.TaskEnvelope {
	return p.tasks
}

func (p *Pool) Active() int64 {
	return p.active.Load()
}

func (p *Pool) Processed() int64 {
	return p.processed.Load()
}
