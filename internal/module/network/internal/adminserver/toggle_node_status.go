package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// ToggleNodeStatus enables or disables a node and drops the node-facing
// caches of its server.
func (s *Service) ToggleNodeStatus(ctx context.Context, req *dto.ToggleNodeStatusRequest) error {
	nodeStore := s.deps.Store.Node()
	data, err := nodeStore.FindOneNode(ctx, req.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("[ToggleNodeStatus] Query Database Error: ", logger.Field("error", err.Error()))
		return xerr.Errorf(xerr.DatabaseQueryError, "[ToggleNodeStatus] Query Database Error")
	}
	data.Enabled = req.Enable

	if err := nodeStore.UpdateNode(ctx, data); err != nil {
		logger.WithContext(ctx).Errorw("[ToggleNodeStatus] Update Database Error: ", logger.Field("error", err.Error()))
		return xerr.Errorf(xerr.DatabaseUpdateError, "[ToggleNodeStatus] Update Database Error")
	}

	return nodeStore.ClearServerCache(ctx, data.ServerId)
}
