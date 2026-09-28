package systemsetting

import (
	"context"
	"encoding/json"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// SetNodeMultiplier stores the node traffic multiplier periods and reloads
// the node subsystem, which evaluates them.
func (s *Service) SetNodeMultiplier(ctx context.Context, req *dto.SetNodeMultiplierRequest) error {
	data, err := json.Marshal(req.Periods)
	if err != nil {
		logger.WithContext(ctx).Errorw("[SetNodeMultiplier] encode the node multiplier config failed", logger.Field("error", err.Error()))
		return xerr.Wrapf(err, xerr.ERROR, "Marshal Node Multiplier Config Error: %s", err.Error())
	}
	if err := s.deps.System.UpdateNodeMultiplierConfig(ctx, string(data)); err != nil {
		logger.WithContext(ctx).Errorw("[SetNodeMultiplier] update the node multiplier config failed", logger.Field("error", err.Error()))
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "Update Node Multiplier Config Error: %s", err.Error())
	}
	return s.deps.reinit("node")
}
