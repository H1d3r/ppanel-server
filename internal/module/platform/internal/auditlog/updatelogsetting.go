package auditlog

import (
	"context"
	"strconv"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/repository/kernel"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateLogSetting stores the log retention settings and, once they are
// committed, propagates them to the running configuration.
func (s *Service) UpdateLogSetting(ctx context.Context, req *dto.LogSetting) error {
	if err := validateLogSetting(req); err != nil {
		return err
	}
	err := s.deps.Store.InPlatformTx(ctx, func(store kernel.PlatformStore) error {
		systemStore := store.System()
		if err := systemStore.UpdateValueByCategoryKey(ctx, "log", "AutoClear", strconv.FormatBool(*req.AutoClear), "bool"); err != nil {
			return err
		}
		return systemStore.UpdateValueByCategoryKey(ctx, "log", "ClearDays", strconv.FormatInt(req.ClearDays, 10), "int64")
	})
	if err != nil {
		logger.WithContext(ctx).Errorw("[UpdateLogSetting] update log setting error", logger.Field("error", err.Error()))
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, " update log setting error: %v", err)
	}

	if s.deps.OnLogSettingChanged != nil {
		s.deps.OnLogSettingChanged(*req.AutoClear, req.ClearDays)
	}
	return nil
}

func validateLogSetting(req *dto.LogSetting) error {
	if req == nil || req.AutoClear == nil || req.ClearDays < 1 || req.ClearDays > 3650 {
		return xerr.Errorf(xerr.InvalidParams, "log retention requires auto_clear and clear_days between 1 and 3650")
	}
	return nil
}
