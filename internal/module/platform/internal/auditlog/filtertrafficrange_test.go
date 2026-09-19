package auditlog

import (
	"context"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/network/entity/traffic"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/platform/internal/repo"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type rangeTrafficReader struct{ repository.TrafficRepo }

func (rangeTrafficReader) QueryServerTrafficRanking(context.Context, time.Time, time.Time) ([]traffic.ServerTrafficRanking, error) {
	return []traffic.ServerTrafficRanking{{ServerId: 1, Total: 100}, {ServerId: 2, Total: 200}, {ServerId: 3, Total: 300}}, nil
}
func (rangeTrafficReader) QueryUserTrafficRanking(context.Context, time.Time, time.Time) ([]traffic.UserTrafficRanking, error) {
	return []traffic.UserTrafficRanking{{UserId: 1, Total: 100}, {UserId: 2, Total: 200}, {UserId: 3, Total: 300}}, nil
}

func TestTrafficRangePagination(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&log.SystemLog{}); err != nil {
		t.Fatal(err)
	}
	today := timeutil.Now().Format(time.DateOnly)
	yesterday := timeutil.Now().AddDate(0, 0, -1).Format(time.DateOnly)
	for _, typ := range []log.Type{log.TypeServerTraffic, log.TypeSubscribeTraffic} {
		for i := 1; i <= 5; i++ {
			if err := db.Create(&log.SystemLog{Type: typ.Uint8(), ObjectID: int64(i), Date: yesterday, Content: `{"total":42}`}).Error; err != nil {
				t.Fatal(err)
			}
		}
		if err := db.Create(&log.SystemLog{Type: typ.Uint8(), Date: today, Content: `{"total":999}`}).Error; err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(Deps{Logs: repo.NewLogRepo(db), Traffic: rangeTrafficReader{}})
	for _, tc := range []struct {
		name   string
		params dto.FilterLogParams
		total  int64
		live   int
	}{
		{"range", dto.FilterLogParams{StartDate: yesterday, EndDate: today}, 8, 3},
		{"today legacy", dto.FilterLogParams{Date: today}, 3, 3},
		{"history legacy", dto.FilterLogParams{Date: yesterday}, 5, 0},
		{"history end", dto.FilterLogParams{EndDate: yesterday}, 5, 0},
		{"today start", dto.FilterLogParams{StartDate: today}, 3, 3},
		{"no filter", dto.FilterLogParams{}, 8, 3},
		{"range precedence", dto.FilterLogParams{Date: today, StartDate: yesterday, EndDate: yesterday}, 5, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, size := range []int{1, 2, 4, 10} {
				count, live := 0, 0
				for page := 1; page <= int(tc.total)/size+2; page++ {
					params := tc.params
					params.Page = page
					params.Size = size
					got, err := service.FilterServerTrafficLog(context.Background(), &dto.FilterServerTrafficLogRequest{FilterLogParams: params})
					if err != nil {
						t.Fatal(err)
					}
					wantLen := min(size, max(0, int(tc.total)-(page-1)*size))
					if got.Total != tc.total || len(got.List) != wantLen {
						t.Fatalf("size=%d page=%d total=%d len=%d", size, page, got.Total, len(got.List))
					}
					for _, row := range got.List {
						count++
						if row.Date == today {
							live++
						}
						if row.Total == 999 {
							t.Fatal("duplicate persisted today")
						}
					}
					users, err := service.FilterUserSubscribeTrafficLog(context.Background(), &dto.FilterSubscribeTrafficRequest{FilterLogParams: params})
					if err != nil || users.Total != tc.total || len(users.List) != wantLen {
						t.Fatalf("user traffic=%+v err=%v", users, err)
					}
				}
				if count != int(tc.total) || live != tc.live {
					t.Fatalf("count=%d live=%d", count, live)
				}
			}
		})
	}
}

type detailsRangeReader struct {
	repository.TrafficRepo
	filter *traffic.TrafficLogDetailsFilter
}

func (r *detailsRangeReader) QueryTrafficLogDetails(_ context.Context, filter *traffic.TrafficLogDetailsFilter) ([]*traffic.TrafficLog, int64, error) {
	r.filter = filter
	return nil, 0, nil
}
func TestTrafficDetailsDateRange(t *testing.T) {
	for _, tc := range []struct {
		name       string
		params     dto.FilterLogParams
		start, end string
	}{
		{"inclusive end", dto.FilterLogParams{StartDate: "2026-01-01", EndDate: "2026-01-03"}, "2026-01-01", "2026-01-04"},
		{"start only", dto.FilterLogParams{StartDate: "2026-01-01", Date: "2025-01-01"}, "2026-01-01", ""},
		{"end only", dto.FilterLogParams{EndDate: "2026-01-03"}, "", "2026-01-04"},
		{"legacy", dto.FilterLogParams{Date: "2026-01-01"}, "2026-01-01", "2026-01-02"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &detailsRangeReader{}
			_, err := NewService(Deps{Traffic: reader}).FilterTrafficLogDetails(context.Background(), &dto.FilterTrafficLogDetailsRequest{FilterLogParams: tc.params})
			if err != nil {
				t.Fatal(err)
			}
			for _, bound := range []struct {
				got  time.Time
				want string
			}{{reader.filter.Start, tc.start}, {reader.filter.End, tc.end}} {
				if bound.want == "" {
					if !bound.got.IsZero() {
						t.Fatal(bound.got)
					}
					continue
				}
				want, err := time.ParseInLocation(time.DateOnly, bound.want, timeutil.Location())
				if err != nil || !bound.got.Equal(want) {
					t.Fatalf("got=%v want=%v err=%v", bound.got, want, err)
				}
			}
		})
	}
}
