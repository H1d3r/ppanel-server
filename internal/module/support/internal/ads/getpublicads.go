package ads

import (
	"context"
	"time"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/support/contract"
	entity "github.com/perfect-panel/server/internal/module/support/entity/ads"
)

// GetPublicAds lists the ads for the public site: enabled and inside their
// schedule, so a scheduled ad does not show early and an expired one stops.
func (s *Service) GetPublicAds(ctx context.Context, req *dto.GetAdsRequest) (resp *dto.GetAdsResponse, err error) {
	// todo: add ads position and device
	status := 1
	// Process-local, like the schedule the admin API writes (time.UnixMilli):
	// a zone-less PostgreSQL timestamp compares wall clocks.
	now := time.Now()
	_, data, err := s.repo.GetAdsListByPage(ctx, 1, 200, entity.Filter{
		Status:   &status,
		ActiveAt: &now,
	})
	if err != nil {
		return nil, err
	}
	resp = &dto.GetAdsResponse{
		List: make([]dto.Ads, len(data)),
	}
	mapping.DeepCopy(&resp.List, data)
	return
}
