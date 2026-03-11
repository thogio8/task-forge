package worker

import (
	"testing"

	"github.com/thogio8/task-forge/internal/model"
)

func BenchmarkPool_Throughput(b *testing.B) {
	pool := NewPool(4, func(model.Task) {}, testLogger)
	task := model.Task{}

	pool.Start()

	b.ResetTimer()
	for range b.N {
		pool.Submit(task)
	}
	pool.Stop()
}

func BenchmarkCalculateBackoff(b *testing.B) {
	for i := range b.N {
		calculateBackoff(i%10 + 1)
	}
}
