package wallet

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/infra/requestctx"
	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
)

type commissionLogRepo struct {
	repository.LogRepo
	logs []*log.SystemLog
}

func (r *commissionLogRepo) FilterSystemLog(_ context.Context, _ *log.FilterParams) ([]*log.SystemLog, int64, error) {
	return r.logs, int64(len(r.logs)), nil
}

// A referrer's commission log must not reveal the referee's order number:
// subscription tokens issued before random tokens were derived from it.
func TestQueryUserCommissionLogHidesRefereeOrderNumbers(t *testing.T) {
	content, err := (&log.Commission{Type: log.CommissionTypePurchase, Amount: 2000, OrderNo: "20241213222445955"}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	repo := &commissionLogRepo{logs: []*log.SystemLog{{Type: log.TypeCommission.Uint8(), ObjectID: 3, Content: string(content)}}}
	ctx := context.WithValue(context.Background(), requestctx.CtxKeyUser, &user.User{Id: 3})

	resp, err := newQueryUserCommissionLogLogic(ctx, Deps{Logs: repo}).QueryUserCommissionLog(&dto.QueryUserCommissionLogListRequest{Page: 1, Size: 10})
	if err != nil {
		t.Fatalf("QueryUserCommissionLog() error = %v", err)
	}
	if len(resp.List) != 1 || resp.List[0].Amount != 2000 {
		t.Fatalf("list = %+v, want the commission entry", resp.List)
	}
	if resp.List[0].OrderNo != "" {
		t.Fatalf("commission log exposed order number %q", resp.List[0].OrderNo)
	}
}
