package adminuser

import (
	"context"

	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"

	"github.com/perfect-panel/server/internal/model/dto"
	"github.com/perfect-panel/server/pkg/logger"
)

type CreateUserAuthMethodLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Create user auth method
func newCreateUserAuthMethodLogic(ctx context.Context, deps Deps) *CreateUserAuthMethodLogic {
	return &CreateUserAuthMethodLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *CreateUserAuthMethodLogic) CreateUserAuthMethod(req *dto.CreateUserAuthMethodRequest) error {
	err := l.deps.Store.InIdentityTx(l.ctx, func(store repository.IdentityStore) error {
		return store.UserAuth().UpsertUserAuthMethod(l.ctx, &user.AuthMethods{
			UserId:         req.UserId,
			AuthType:       req.AuthType,
			AuthIdentifier: req.AuthIdentifier,
		})
	})
	if err != nil {
		l.Errorw("[CreateUserAuthMethodLogic] Create User Auth Method Error:", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "Create User Auth Method Error")
	}
	return nil
}
