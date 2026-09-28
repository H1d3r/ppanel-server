package systemsetting

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateNodeConfig stores the node settings, kept in the server settings
// category, and reloads the node subsystem.
func (s *Service) UpdateNodeConfig(ctx context.Context, req *dto.NodeConfig) error {
	if err := updateConfigFields(ctx, s.deps, "server", convertedConfigFields(*req)); err != nil {
		logger.WithContext(ctx).Errorw("[UpdateNodeConfig] update node config error", logger.Field("error", err.Error()))
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update server config error: %v", err)
	}
	return s.deps.reinit("node")
}
