package kafka

import (
	"context"

	kafkago "github.com/segmentio/kafka-go"
)

type Producer struct {
	writeFn func(ctx context.Context, key string, value []byte, headers []kafkago.Header) error
	closeFn func() error
}

func NewProducer(brokers []string, topic string) *Producer {
	w := &kafkago.Writer{
		Addr:     kafkago.TCP(brokers...),
		Topic:    topic,
		Balancer: &kafkago.Hash{},
	}

	return &Producer{
		writeFn: func(ctx context.Context, key string, value []byte, headers []kafkago.Header) error {
			return w.WriteMessages(ctx, kafkago.Message{
				Key:     []byte(key),
				Value:   value,
				Headers: headers,
			})
		},
		closeFn: w.Close,
	}
}

func (p *Producer) Publish(ctx context.Context, key string, value []byte, headers ...kafkago.Header) error {
	return p.writeFn(ctx, key, value, headers)
}

func (p *Producer) Close() error {
	return p.closeFn()
}
