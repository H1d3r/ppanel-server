// Package subscription holds the queue handlers of the scheduled
// subscription tasks: the lifecycle sweep and the pre-expiry reminder. The
// business rules live in the subscription module; the handlers only run it.
package subscription

import (
	"context"

	"github.com/hibiken/asynq"
	module "github.com/perfect-panel/server/internal/module/subscription"
)

// CheckSubscriptionHandler is the queue shell of the subscription lifecycle
// sweep.
type CheckSubscriptionHandler struct {
	service module.Service
}

// NewCheckSubscriptionHandler builds the shell over the subscription facade.
func NewCheckSubscriptionHandler(service module.Service) *CheckSubscriptionHandler {
	return &CheckSubscriptionHandler{service: service}
}

func (h *CheckSubscriptionHandler) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	return h.service.CheckSubscriptions(ctx)
}
