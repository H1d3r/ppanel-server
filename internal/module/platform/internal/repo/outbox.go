package repo

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/module/platform/entity/outbox"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

var _ repository.OutboxRepo = (*outboxRepo)(nil)

type outboxRepo struct {
	db *gorm.DB
}

// NewOutboxRepo builds the module-owned implementation.
func NewOutboxRepo(db *gorm.DB) repository.OutboxRepo {
	return &outboxRepo{db: db}
}

func (m *outboxRepo) Append(ctx context.Context, topic, eventKey, payload string) error {
	return m.db.WithContext(ctx).Create(&outbox.Event{
		Topic:    topic,
		EventKey: eventKey,
		Payload:  payload,
	}).Error
}

func (m *outboxRepo) ListUnpublished(ctx context.Context, limit int) ([]*outbox.Event, error) {
	var events []*outbox.Event
	err := m.db.WithContext(ctx).
		Where("published_at IS NULL").
		Order("id ASC").
		Limit(limit).
		Find(&events).Error
	return events, err
}

func (m *outboxRepo) MarkPublished(ctx context.Context, id int64) error {
	return m.db.WithContext(ctx).Model(&outbox.Event{}).
		Where("id = ?", id).
		Update("published_at", time.Now()).Error
}

func (m *outboxRepo) DeletePublishedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result := m.db.WithContext(ctx).
		Where("published_at IS NOT NULL AND published_at < ?", cutoff).
		Delete(&outbox.Event{})
	return result.RowsAffected, result.Error
}
