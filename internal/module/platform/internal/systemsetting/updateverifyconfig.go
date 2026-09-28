package systemsetting

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateVerifyConfig stores the verification settings and reloads the verify
// subsystem from them.
func (s *Service) UpdateVerifyConfig(ctx context.Context, req *dto.VerifyConfig) error {
	if err := updateConfigFields(ctx, s.deps, "verify", convertedConfigFields(*req)); err != nil {
		logger.WithContext(ctx).Errorw("[UpdateVerifyConfig] update verify config error", logger.Field("error", err.Error()))
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update verify config error: %v", err)
	}
	return s.deps.reinit("verify")
}
