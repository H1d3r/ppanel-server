// Package storefront implements the public subscription listings of the
// subscription module. Only the module facade may reach it.
package storefront

import (
	"context"

	"github.com/perfect-panel/server/internal/module/network/entity/node"
	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/repository"
)

// NodeLister is the network read port listing the nodes a plan selects; the
// legacy node repository satisfies it structurally.
type NodeLister interface {
	ListNodesByScope(ctx context.Context, nodeIDs []int64, tags []string, enabled *bool, preload bool) ([]*node.Node, error)
}

type Deps struct {
	Plans    repository.SubscribeRepo
	UserSubs repository.UserSubscriptionRepo
	Nodes    NodeLister
	// IsTrialPlan reports whether the plan is the currently configured trial
	// plan; the registration config is runtime-mutable.
	IsTrialPlan func(planID int64) bool
}

func (d Deps) isTrialPlan(planID int64) bool {
	return d.IsTrialPlan != nil && d.IsTrialPlan(planID)
}

type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) QuerySubscribeList(ctx context.Context, req *dto.QuerySubscribeListRequest) (*dto.QuerySubscribeListResponse, error) {
	return newQuerySubscribeListLogic(ctx, s.deps).QuerySubscribeList(req)
}

func (s *Service) QuerySubscribeGroupList(ctx context.Context) (*dto.QuerySubscribeGroupListResponse, error) {
	return newQuerySubscribeGroupListLogic(ctx, s.deps).QuerySubscribeGroupList()
}
