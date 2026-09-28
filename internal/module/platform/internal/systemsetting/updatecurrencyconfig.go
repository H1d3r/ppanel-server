package systemsetting

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateCurrencyConfig stores the currency settings and, once they are
// stored, reloads the currency subsystem: a failed write must not reload a
// configuration that did not change.
func (s *Service) UpdateCurrencyConfig(ctx context.Context, req *dto.CurrencyConfig) error {
	if err := updateConfigFields(ctx, s.deps, "currency", convertedConfigFields(*req)); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update currency config: %v", err)
	}
	return s.deps.reinit("currency")
}
