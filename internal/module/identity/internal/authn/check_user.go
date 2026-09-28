package authn

import (
	"context"
	"errors"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

// CheckUser reports whether an account signs in with the email address.
func (s *Service) CheckUser(ctx context.Context, req *dto.CheckUserRequest) (*dto.CheckUserResponse, error) {
	exist, err := s.identityExists(ctx, identifier.Email, req.Email)
	if err != nil {
		return nil, err
	}
	return &dto.CheckUserResponse{Exist: exist}, nil
}

func (s *Service) identityExists(ctx context.Context, authType, authIdentifier string) (bool, error) {
	method, err := s.deps.Store.UserAuth().FindUserAuthMethodByOpenID(ctx, authType, authIdentifier)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, xerr.Wrapf(err, xerr.DatabaseQueryError, "find %s identity", authType)
	}
	return err == nil && method.UserId != 0, nil
}
