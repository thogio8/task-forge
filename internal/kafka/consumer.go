package kafka

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/thogio8/task-forge/internal/model"
)

type MessageReader interface {
	FetchMessage(ctx context.Context) (kafkago.Message, error)
	CommitMessages(ctx context.Context, msg ...kafkago.Message) error
	Close() error
}

type TaskClaimer interface {
	ClaimTask(ctx context.Context, workerID string, taskID uuid.UUID) (*model.Task, error)
}

type Consumer struct {
	reader   MessageReader
	claimer  TaskClaimer
	tasks    chan<- model.Task
	workerID string
	logger   *slog.Logger
	done     chan struct{}
}

func NewConsumer(brokers []string, topic string, groupID string, claimer TaskClaimer, tasks chan<- model.Task, workerID string, logger *slog.Logger) *Consumer {
	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: brokers,
		GroupID: groupID,
		Topic:   topic,
	})

	return &Consumer{
		reader:   reader,
		claimer:  claimer,
		tasks:    tasks,
		workerID: workerID,
		logger:   logger,
		done:     make(chan struct{}),
	}
}

func NewConsumerWithReader(reader MessageReader, claimer TaskClaimer, tasks chan<- model.Task, workerID string, logger *slog.Logger) *Consumer {
	return &Consumer{
		reader:   reader,
		claimer:  claimer,
		tasks:    tasks,
		workerID: workerID,
		logger:   logger,
		done:     make(chan struct{}),
	}
}

func (c *Consumer) Run(ctx context.Context) {
	c.logger.Info("kafka consumer started", "worker_id", c.workerID)

	for {
		msg, err := c.reader.FetchMessage(ctx)

		if err != nil {
			if ctx.Err() != nil {
				close(c.done)
				c.logger.Info("kafka consumer stopped", "worker_id", c.workerID)
				return
			}

			c.logger.Error("failed to fetch kafka message", "worker_id", c.workerID, "error", err)
			continue
		}

		c.processMessage(ctx, msg)
	}
}

func (c *Consumer) Stop() {
	<-c.done
	err := c.reader.Close()

	if err != nil {
		c.logger.Error("failed to close reader", "worker_id", c.workerID, "error", err)
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg kafkago.Message) {
	var data struct {
		TaskID string `json:"task_id"`
	}

	if err := json.Unmarshal(msg.Value, &data); err != nil {
		c.logger.Error("failed to unmarshal kafka message", "error", err)
		if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
			c.logger.Warn("failed to commit kafka message", "error", commitErr)
		}
		return
	}

	taskID, err := uuid.Parse(data.TaskID)
	if err != nil {
		c.logger.Error("invalid task ID in kafka message", "task_id", data.TaskID, "error", err)
		if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
			c.logger.Warn("failed to commit kafka message", "error", commitErr)
		}
		return
	}

	task, err := c.claimer.ClaimTask(ctx, c.workerID, taskID)
	if err != nil {
		c.logger.Error("failed to claim task from kafka event", "task_id", taskID, "error", err)
		return
	}

	if task == nil {
		c.logger.Debug("task already claimed or not found", "task_id", taskID)
		if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
			c.logger.Warn("failed to commit kafka message", "error", commitErr)
		}
		return
	}

	c.tasks <- *task
	if commitErr := c.reader.CommitMessages(ctx, msg); commitErr != nil {
		c.logger.Warn("failed to commit kafka message", "error", commitErr)
	}
	c.logger.Info("task dispatched from kafka", "task_id", taskID)
}
