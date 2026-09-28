package systemsetting

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateRegisterConfig stores the registration settings and reloads the
// register subsystem.
func (s *Service) UpdateRegisterConfig(ctx context.Context, req *dto.RegisterConfig) error {
	if err := updateConfigFields(ctx, s.deps, "register", convertedConfigFields(*req)); err != nil {
		logger.WithContext(ctx).Errorw("[UpdateRegisterConfig] update register config error", logger.Field("error", err.Error()))
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update register config error: %v", err.Error())
	}
	return s.deps.reinit("register")
}
