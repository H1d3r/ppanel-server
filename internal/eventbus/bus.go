// Package eventbus is the in-process domain-event dispatcher (ADR-001
// step-6 preparation). Producers append events to the outbox inside their
// domain transaction; the dispatcher drains unpublished events and delivers
// each to every subscriber of its topic. Delivery is at-least-once: an
// event is marked published only after every subscriber succeeded, and
// subscribers are expected to be idempotent (the inbox pattern). Replacing
// this dispatcher with a message broker changes only the driver: topics map
// to broker topics and subscribers to consumer groups.
package eventbus

import (
	"context"
	"fmt"

	"github.com/perfect-panel/server/internal/model/entity/outbox"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

// Event is the payload handed to subscribers.
type Event struct {
	ID      int64
	Topic   string
	Key     string
	Payload string
}

// Handler processes one event. Returning an error keeps the event
// unpublished so the next dispatch tick retries it; handlers must therefore
// be idempotent.
type Handler func(ctx context.Context, event Event) error

type subscription struct {
	consumer string
	handler  Handler
}

// Bus routes outbox events to subscribers by topic.
type Bus struct {
	outbox      repository.OutboxRepo
	subscribers map[string][]subscription
}

func New(outboxRepo repository.OutboxRepo) *Bus {
	return &Bus{
		outbox:      outboxRepo,
		subscribers: make(map[string][]subscription),
	}
}

// Subscribe registers a consumer for a topic. The consumer name is the
// subscriber's identity for logging and mirrors the inbox consumer it uses
// for idempotency. Registration happens at composition time, before
// dispatching starts; Subscribe is not safe for concurrent use with
// Dispatch.
func (b *Bus) Subscribe(topic, consumer string, handler Handler) {
	b.subscribers[topic] = append(b.subscribers[topic], subscription{consumer: consumer, handler: handler})
}

// Dispatch drains up to limit unpublished events, delivering each to every
// subscriber of its topic. An event is marked published only when all its
// subscribers succeed; a failing subscriber leaves the event unpublished
// for the next tick (subscribers that already succeeded re-run and skip via
// their inbox markers). Events without subscribers are marked published
// immediately so an orphaned topic cannot wedge the queue.
func (b *Bus) Dispatch(ctx context.Context, limit int) error {
	events, err := b.outbox.ListUnpublished(ctx, limit)
	if err != nil {
		return err
	}
	for _, row := range events {
		if err := b.deliver(ctx, row); err != nil {
			// Keep ordering per event id: stop this tick on the first
			// failure so retries preserve arrival order.
			return err
		}
		if err := b.outbox.MarkPublished(ctx, row.ID); err != nil {
			return err
		}
	}
	return nil
}

func (b *Bus) deliver(ctx context.Context, row *outbox.Event) error {
	event := Event{ID: row.ID, Topic: row.Topic, Key: row.EventKey, Payload: row.Payload}
	for _, sub := range b.subscribers[row.Topic] {
		if err := sub.handler(ctx, event); err != nil {
			logger.WithContext(ctx).Errorw("[EventBus] subscriber failed; event stays queued",
				logger.Field("topic", row.Topic), logger.Field("key", row.EventKey),
				logger.Field("consumer", sub.consumer), logger.Field("error", err.Error()))
			return fmt.Errorf("subscriber %s on %s: %w", sub.consumer, row.Topic, err)
		}
	}
	return nil
}
