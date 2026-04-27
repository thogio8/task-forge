package worker

import (
	"context"
	"testing"

	"github.com/thogio8/task-forge/internal/model"
)

func BenchmarkPool_Throughput(b *testing.B) {
	pool := NewPool(4, func(context.Context, model.Task) {}, testLogger)
	task := model.Task{}
	ctx := context.Background()

	pool.Start()

	b.ResetTimer()
	for range b.N {
		pool.Submit(ctx, task)
	}
	pool.Stop()
}

func BenchmarkCalculateBackoff(b *testing.B) {
	for i := range b.N {
		calculateBackoff(i%10 + 1)
	}
}
