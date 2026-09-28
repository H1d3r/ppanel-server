package profile

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

// GetOAuthMethods lists the calling account's identities as stored.
func (s *Service) GetOAuthMethods(ctx context.Context) (*dto.GetOAuthMethodsResponse, error) {
	u, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	methods, err := s.deps.UserAuth.FindUserAuthMethods(ctx, u.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("find user auth methods failed:", logger.Field("error", err.Error()))
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find user auth methods failed: %v", err.Error())
	}
	list := make([]dto.UserAuthMethod, 0, len(methods))
	for _, method := range methods {
		list = append(list, dto.UserAuthMethod{
			AuthType:       method.AuthType,
			AuthIdentifier: method.AuthIdentifier,
			Verified:       method.Verified,
		})
	}
	return &dto.GetOAuthMethodsResponse{
		Methods: list,
	}, nil
}
