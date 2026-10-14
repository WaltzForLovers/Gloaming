package events

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/segmentio/kafka-go"
)

type Bus struct {
	addr   string
	writer *kafka.Writer
}

func Open(addr string) (*Bus, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	conn, err := kafka.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	conn.Close()
	return &Bus{
		addr: addr,
		writer: &kafka.Writer{
			Addr:                   kafka.TCP(addr),
			AllowAutoTopicCreation: true,
			Balancer:               &kafka.LeastBytes{},
			BatchTimeout:           10 * time.Millisecond,
		},
	}, nil
}

func (b *Bus) Publish(ctx context.Context, topic string, key, value []byte) error {
	if err := b.ensure(topic); err != nil {
		return err
	}
	return b.writer.WriteMessages(ctx, kafka.Message{Topic: topic, Key: key, Value: value})
}

func (b *Bus) Ensure(topic string) error { return b.ensure(topic) }

func (b *Bus) ensure(topic string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := kafka.DialContext(ctx, "tcp", b.addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	err = conn.CreateTopics(kafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1})
	var kerr kafka.Error
	if err == nil || (errors.As(err, &kerr) && kerr == kafka.TopicAlreadyExists) {
		return nil
	}
	return err
}

func (b *Bus) Consume(ctx context.Context, topic, group string, handle func(context.Context, []byte) error) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{b.addr},
		Topic:       topic,
		GroupID:     group,
		MinBytes:    1,
		MaxBytes:    1e6,
		MaxWait:     500 * time.Millisecond,
		StartOffset: kafka.FirstOffset,
	})
	go func() {
		defer reader.Close()
		for {
			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				log.Printf("kafka %s: %v", topic, err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				continue
			}
			hctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := handle(hctx, msg.Value); err != nil {
				log.Printf("kafka %s: %v", topic, err)
			}
			cancel()
		}
	}()
}
