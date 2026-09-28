// Service assembly for the edge subdomain of the network module: the token-
// authenticated client manifest. Only the module facade may reach it.
package edge

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/entity/node"
	"github.com/perfect-panel/server/internal/module/subscription/entity/subscribe"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
)

// Snapshot is the per-request view of the runtime-mutable settings the edge
// manifest consumes.
type Snapshot struct {
	Subscribe config.SubscribeConfig
}

// Deps declares the subdomain's dependencies; the module facade reads them
// from the store (DepsFrom).
type Deps struct {
	// Subscriptions resolves the manifest token, Accounts gates on the
	// owner's account, Plans and Nodes select the proxies.
	Subscriptions TokenResolver
	Accounts      AccountStateReader
	Plans         PlanReader
	Nodes         NodeLister
	// Config snapshots the runtime-mutable settings per request.
	Config func() Snapshot
}

// TokenResolver finds the subscription behind a manifest token.
type TokenResolver interface {
	FindOneSubscribeByToken(ctx context.Context, token string) (*usersub.Subscribe, error)
}

// AccountStateReader reads the subscription owner's account state.
type AccountStateReader interface {
	FindAccountState(ctx context.Context, id int64) (*user.AccountState, error)
}

// PlanReader reads the subscription's plan.
type PlanReader interface {
	FindOne(ctx context.Context, id int64) (*subscribe.Subscribe, error)
}

// NodeLister lists the nodes a plan's scope selects.
type NodeLister interface {
	ListNodesByScope(ctx context.Context, nodeIDs []int64, tags []string, enabled *bool, preload bool) ([]*node.Node, error)
}

// Service is the edge manifest entry point used by the network facade.
type Service struct {
	deps Deps
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) Manifest(ctx context.Context, token string) (*dto.EdgeManifestResponse, error) {
	return newManifestLogic(ctx, s.deps).Manifest(token)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	Node() repository.NodeRepo
	Subscribe() repository.SubscribeRepo
	User() repository.UserRepo
	UserSubscription() repository.UserSubscriptionRepo
}

// DepsFrom reads the manifest's ports from store.
func DepsFrom(store Store, config func() Snapshot) Deps {
	return Deps{
		Subscriptions: store.UserSubscription(),
		Accounts:      store.User(),
		Plans:         store.Subscribe(),
		Nodes:         store.Node(),
		Config:        config,
	}
}
