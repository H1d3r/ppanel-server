package traffic

import (
	"time"

	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/internal/trafficagg"
)

// aggregatorDeps builds the traffic aggregator's dependencies from the queue
// worker's service context.
func aggregatorDeps(s *svc.ServiceContext) trafficagg.Deps {
	return trafficagg.Deps{
		Store: s.Store,
		Redis: s.Redis,
		TrafficReportThreshold: func() int64 {
			return s.Config.Node.TrafficReportThreshold
		},
		Multiplier: func(at time.Time) float32 {
			if s.NodeMultiplierManager == nil {
				return 1
			}
			return s.NodeMultiplierManager.GetMultiplier(at)
		},
	}
}
