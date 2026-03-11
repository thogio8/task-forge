package worker

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/thogio8/task-forge/internal/model"
)

type Pool struct {
	workerCount int
	processFunc func(model.Task)
	tasks       chan model.Task
	wg          sync.WaitGroup
	logger      *slog.Logger
	processed   atomic.Int64
	active      atomic.Int64
}

func NewPool(workerCount int, processFunc func(model.Task), logger *slog.Logger) *Pool {
	bufferedChannel := make(chan model.Task, workerCount*2)

	return &Pool{workerCount: workerCount, processFunc: processFunc, tasks: bufferedChannel, logger: logger}
}

func (p *Pool) Start() {
	p.wg.Add(p.workerCount)

	for i := range p.workerCount {
		go func(_ int) {
			defer p.wg.Done()
			for task := range p.tasks {
				p.active.Add(1)
				p.processFunc(task)
				p.active.Add(-1)
				p.processed.Add(1)
			}
		}(i)
	}
}

func (p *Pool) Submit(task model.Task) {
	p.tasks <- task
}

func (p *Pool) Stop() {
	close(p.tasks)
	p.wg.Wait()
}

func (p *Pool) Tasks() chan model.Task {
	return p.tasks
}

func (p *Pool) Active() int64 {
	return p.active.Load()
}

func (p *Pool) Processed() int64 {
	return p.processed.Load()
}
