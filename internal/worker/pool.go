package worker

import (
	"log/slog"
	"sync"

	"github.com/thogio8/task-forge/internal/model"
)

type Pool struct {
	workerCount int
	processFunc func(model.Task)
	tasks       chan model.Task
	wg          sync.WaitGroup
	logger      *slog.Logger
}

func NewPool(workerCount int, processFunc func(model.Task), logger *slog.Logger) *Pool {
	bufferedChannel := make(chan model.Task, workerCount*2)

	return &Pool{workerCount: workerCount, processFunc: processFunc, tasks: bufferedChannel, logger: logger}
}

func (p *Pool) Start() {
	p.wg.Add(p.workerCount)

	for i := range p.workerCount {
		go func(index int) {
			defer p.wg.Done()
			for task := range p.tasks {
				p.processFunc(task)
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
