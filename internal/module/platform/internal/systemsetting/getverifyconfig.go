package systemsetting

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/pkg/xerr"
)

// GetVerifyConfig returns the stored verification settings. It only reads:
// the running configuration is re-initialized when the settings are updated.
func (s *Service) GetVerifyConfig(ctx context.Context) (*dto.VerifyConfig, error) {
	rows, err := s.deps.System.GetVerifyConfig(ctx)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "get verify config: %v", err)
	}
	resp := &dto.VerifyConfig{}
	config.SystemConfigSliceReflectToStruct(rows, resp)
	return resp, nil
}
