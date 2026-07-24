package subscription

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/svc"
)

// CheckSubscriptionLogic is the queue shell for the subscription lifecycle
// sweep; the business logic lives in the subscription module (ADR-001
// step 6 preparation).
type CheckSubscriptionLogic struct {
	svc *svc.ServiceContext
}

func NewCheckSubscriptionLogic(svc *svc.ServiceContext) *CheckSubscriptionLogic {
	return &CheckSubscriptionLogic{
		svc: svc,
	}
}

func (l *CheckSubscriptionLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	return l.svc.Subscription.CheckSubscriptions(ctx)
}
