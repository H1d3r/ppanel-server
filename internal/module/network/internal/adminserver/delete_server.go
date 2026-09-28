package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// DeleteServer removes a server together with its node configuration
// override, in one network transaction, and then drops its node-facing
// caches.
func (s *Service) DeleteServer(ctx context.Context, req *dto.DeleteServerRequest) error {
	if err := s.deps.Store.InNetworkTx(ctx, func(store repository.NetworkStore) error {
		nodeStore := store.Node()
		if err := nodeStore.DeleteServer(ctx, req.Id); err != nil {
			return err
		}
		return nodeStore.DeleteServerConfigOverride(ctx, req.Id)
	}); err != nil {
		logger.WithContext(ctx).Errorw("[DeleteServer] Delete Server Error: ", logger.Field("error", err.Error()))
		return xerr.Errorf(xerr.DatabaseDeletedError, "[DeleteServer] Delete Server Error")
	}
	return s.deps.Store.Node().ClearServerCache(ctx, req.Id)
}
