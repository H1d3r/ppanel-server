package plan

import (
	"context"
	"errors"

	dto "github.com/perfect-panel/server/internal/module/subscription/contract"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// DeleteSubscribe deletes a plan no active user subscription holds. The
// check and the delete share one transaction.
func (s *Service) DeleteSubscribe(ctx context.Context, req *dto.DeleteSubscribeRequest) error {
	// phase tells a failed check from a failed delete for the error code.
	phase := "check"
	err := s.deps.Store.InSubscriptionTx(ctx, func(store repository.SubscriptionStore) error {
		total, err := store.UserSubscription().CountUserSubscribesBySubscribeIdAndStatus(ctx, req.Id, int64(usersub.SubscribeStatusActive))
		if err != nil {
			return err
		}
		if total != 0 {
			return errorIsExistActiveUser
		}
		phase = "delete"
		return store.Subscribe().Delete(ctx, req.Id)
	})
	if err != nil {
		if errors.Is(err, errorIsExistActiveUser) {
			return xerr.Errorf(xerr.SubscribeIsUsedError, "subscribe is used")
		}
		log := logger.WithContext(ctx)
		if phase == "delete" {
			log.Error("[DeleteSubscribeLogic] delete subscribe failed: ", logger.Field("error", err.Error()))
			return xerr.Wrapf(err, xerr.DatabaseDeletedError, "delete subscribe failed: %v", err.Error())
		}
		log.Error("[DeleteSubscribeLogic] check subscribe failed: ", logger.Field("error", err.Error()))
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "check subscribe failed: %v", err.Error())
	}
	return nil
}
