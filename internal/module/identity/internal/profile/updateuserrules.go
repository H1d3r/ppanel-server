package profile

import (
	"context"
	"encoding/json"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type UpdateUserRulesLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewUpdateUserRulesLogic Update User Rules
func newUpdateUserRulesLogic(ctx context.Context, deps Deps) *UpdateUserRulesLogic {
	return &UpdateUserRulesLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *UpdateUserRulesLogic) UpdateUserRules(req *dto.UpdateUserRulesRequest) error {
	u, ok := user.FromContext(l.ctx)
	if !ok {
		logger.Error("current user is not found in context")
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	if len(req.Rules) > 0 {
		bytes, err := json.Marshal(req.Rules)
		if err != nil {
			return xerr.Wrapf(err, xerr.ERROR, "marshal rules")
		}
		err = l.deps.Users.UpdateColumns(l.ctx, u.Id, map[string]interface{}{"rules": string(bytes)})
		if err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update rules of user %d", u.Id)
		}
	}
	return nil
}
