package orm

import (
	"context"
	"time"
)

// BatchDeleter deletes up to limit rows older than before per call and
// reports how many it deleted.
type BatchDeleter interface {
	DeleteBeforeBatch(ctx context.Context, before time.Time, limit int) (int64, error)
}

// DeleteBefore deletes the rows older than before batch by batch, so a large
// backlog never runs as one long statement holding its locks, and returns
// how many rows it deleted, including those of the batches before a failure.
func DeleteBefore(ctx context.Context, rows BatchDeleter, before time.Time, batchSize int) (int64, error) {
	var total int64
	for {
		deleted, err := rows.DeleteBeforeBatch(ctx, before, batchSize)
		total += deleted
		if err != nil || deleted < int64(batchSize) {
			return total, err
		}
	}
}
