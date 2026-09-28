// Service assembly for the public-info subdomain: unauthenticated site-level
// reads (global configuration, ToS/privacy, aggregate stats, client
// downloads, liveness). Only the module facade may reach it.
package publicinfo

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"golang.org/x/sync/singleflight"
)

// Service is the public-info subdomain entry point used by the platform
// facade.
type Service struct {
	deps Deps
	// statRefresh collapses concurrent refreshes of the site statistics.
	statRefresh singleflight.Group
}

func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}

func (s *Service) GetGlobalConfig(ctx context.Context) (*dto.GetGlobalConfigResponse, error) {
	return NewGetGlobalConfigLogic(ctx, GetGlobalConfigDependencies{
		Store:  s.deps.Store,
		Config: s.deps.Config(),
	}).GetGlobalConfig()
}

func (s *Service) GetTos(ctx context.Context) (*dto.GetTosResponse, error) {
	return newGetTosLogic(ctx, s.deps).GetTos()
}

func (s *Service) GetPrivacyPolicy(ctx context.Context) (*dto.PrivacyPolicyConfig, error) {
	return newGetPrivacyPolicyLogic(ctx, s.deps).GetPrivacyPolicy()
}

func (s *Service) GetClient(ctx context.Context) (*dto.GetSubscribeClientResponse, error) {
	return newGetClientLogic(ctx, s.deps).GetClient()
}

func (s *Service) Heartbeat(ctx context.Context) (*dto.HeartbeatResponse, error) {
	return newHeartbeatLogic(ctx, s.deps).Heartbeat()
}
