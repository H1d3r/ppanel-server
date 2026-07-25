// Package outbox holds the generic domain-event outbox row (ADR-001 step-6
// preparation): appended in the owning domain's transaction, delivered by
// the in-process dispatcher, consumed idempotently via the inbox.
package outbox

import "time"

type Event struct {
	ID          int64  `gorm:"primaryKey;autoIncrement"`
	Topic       string `gorm:"type:varchar(64);not null"`
	EventKey    string `gorm:"type:varchar(191);not null"`
	Payload     string `gorm:"type:text;not null"`
	CreatedAt   time.Time
	PublishedAt *time.Time
}

func (Event) TableName() string {
	return "domain_event_outbox"
}
