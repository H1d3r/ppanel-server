package traffic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/config"
)

// cleanupRepo records the batch deletions of one table, deleting one row
// per batch or failing with err.
type cleanupRepo struct {
	threshold time.Time
	err       error
	calls     int
}

var _ batchDeleter = (*cleanupRepo)(nil)

func (r *cleanupRepo) DeleteBeforeBatch(_ context.Context, threshold time.Time, _ int) (int64, error) {
	r.threshold = threshold
	r.calls++
	return 1, r.err
}

func TestLogCleanupRunsIndependentlyAndPropagatesFailures(t *testing.T) {
	trafficRepo := &cleanupRepo{}
	logRepo := &cleanupRepo{}
	logic := newLogCleanupLogic(trafficRepo, logRepo, func() config.Log { return config.Log{AutoClear: true, ClearDays: 7} })

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if trafficRepo.calls != 1 || logRepo.calls != 1 || trafficRepo.threshold.IsZero() || !trafficRepo.threshold.Equal(logRepo.threshold) {
		t.Fatalf("cleanup batches = %d/%d, thresholds=%v/%v", trafficRepo.calls, logRepo.calls, trafficRepo.threshold, logRepo.threshold)
	}

	trafficRepo.err = errors.New("delete failed")
	if err := logic.ProcessTask(context.Background(), nil); !errors.Is(err, trafficRepo.err) {
		t.Fatalf("cleanup error = %v, want %v", err, trafficRepo.err)
	}
}

func TestLogCleanupRejectsUnsafeRetentionWithoutDeleting(t *testing.T) {
	trafficRepo := &cleanupRepo{}
	logRepo := &cleanupRepo{}
	logic := newLogCleanupLogic(trafficRepo, logRepo, func() config.Log { return config.Log{AutoClear: true, ClearDays: -1} })
	if err := logic.ProcessTask(context.Background(), nil); err == nil {
		t.Fatal("unsafe retention was accepted")
	}
	if trafficRepo.calls != 0 || logRepo.calls != 0 {
		t.Fatalf("unsafe cleanup deleted batches: %d/%d", trafficRepo.calls, logRepo.calls)
	}
}
