package subscription

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/svc"
)

// RemindExpiringLogic is the queue shell for the pre-expiry reminder; the
// business logic lives in the subscription module.
type RemindExpiringLogic struct {
	svc *svc.ServiceContext
}

func NewRemindExpiringLogic(svc *svc.ServiceContext) *RemindExpiringLogic {
	return &RemindExpiringLogic{svc: svc}
}

func (l *RemindExpiringLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	return l.svc.Subscription.RemindExpiringSubscriptions(ctx)
}
