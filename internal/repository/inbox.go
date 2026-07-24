package repository

import (
	"context"
	"errors"
	"time"

	"github.com/perfect-panel/server/internal/model/entity/inbox"
	"gorm.io/gorm"
)

// InboxRepo is the idempotent-consumer inbox (ADR-001 step 2): a domain step
// records that it processed an event inside its own transaction, so
// at-least-once deliveries and reconciliation replays never apply the same
// mutation twice.
type InboxRepo interface {
	// Find returns the processed marker, or (nil, nil) when the step has not
	// run yet.
	Find(ctx context.Context, consumer, eventKey string) (*inbox.Record, error)
	// Insert records the step as processed. It must run inside the same
	// transaction as the step's mutations; a duplicate-key error means a
	// concurrent delivery won the race and this transaction must roll back.
	Insert(ctx context.Context, consumer, eventKey, result string) error
	// DeleteProcessedBefore removes markers older than the replay contract;
	// every flow that consults the inbox resolves well inside the retention
	// window (deferred closes in minutes, bucket replays in hours).
	DeleteProcessedBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

var _ InboxRepo = (*inboxRepo)(nil)

type inboxRepo struct {
	db *gorm.DB
}

func newInboxRepo(db *gorm.DB) InboxRepo {
	return &inboxRepo{db: db}
}

func (m *inboxRepo) Find(ctx context.Context, consumer, eventKey string) (*inbox.Record, error) {
	var record inbox.Record
	err := m.db.WithContext(ctx).
		Where("consumer = ? AND event_key = ?", consumer, eventKey).
		First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (m *inboxRepo) Insert(ctx context.Context, consumer, eventKey, result string) error {
	return m.db.WithContext(ctx).Create(&inbox.Record{
		Consumer: consumer,
		EventKey: eventKey,
		Result:   result,
	}).Error
}

func (m *inboxRepo) DeleteProcessedBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	result := m.db.WithContext(ctx).
		Where("processed_at < ?", cutoff).
		Delete(&inbox.Record{})
	return result.RowsAffected, result.Error
}
