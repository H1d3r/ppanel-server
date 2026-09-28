package traffic

import (
	"context"
	"testing"
	"time"

	trafficEntity "github.com/perfect-panel/server/internal/module/network/entity/traffic"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
)

// statTrafficRepo ranks one subscription and one server.
type statTrafficRepo struct {
	queries int
}

var _ trafficRankings = (*statTrafficRepo)(nil)

func (r *statTrafficRepo) QueryUserTrafficRanking(context.Context, time.Time, time.Time) ([]trafficEntity.UserTrafficRanking, error) {
	r.queries++
	return []trafficEntity.UserTrafficRanking{{SubscribeId: 9, UserId: 3, Upload: 5, Download: 7, Total: 12}}, nil
}

func (r *statTrafficRepo) QueryServerTrafficRanking(context.Context, time.Time, time.Time) ([]trafficEntity.ServerTrafficRanking, error) {
	return []trafficEntity.ServerTrafficRanking{{ServerId: 4, Upload: 5, Download: 7, Total: 12}}, nil
}

// statLogRepo holds the recorded rows.
type statLogRepo struct {
	rows []*log.SystemLog
}

var _ statLogs = (*statLogRepo)(nil)

func (r *statLogRepo) InsertBatch(_ context.Context, rows []*log.SystemLog, _ int) error {
	r.rows = append(r.rows, rows...)
	return nil
}

func (r *statLogRepo) FindFirstByDateType(_ context.Context, date string, typ uint8) (*log.SystemLog, error) {
	for _, row := range r.rows {
		if row.Date == date && row.Type == typ {
			return row, nil
		}
	}
	return nil, nil
}

// A second run for the same day (another replica's tick, or a replay) must
// not insert the day's statistics again.
func TestStatLogicRecordsEachDayOnce(t *testing.T) {
	rankings, logs := &statTrafficRepo{}, &statLogRepo{}
	logic := newStatLogic(logs, rankings)
	for range 2 {
		if err := logic.ProcessTask(context.Background(), nil); err != nil {
			t.Fatal(err)
		}
	}
	// One subscriber row, one server row and the three daily summaries.
	if len(logs.rows) != 5 || rankings.queries != 1 {
		t.Fatalf("rows = %d, ranking queries = %d; want one day's 5 rows from one run", len(logs.rows), rankings.queries)
	}
}
