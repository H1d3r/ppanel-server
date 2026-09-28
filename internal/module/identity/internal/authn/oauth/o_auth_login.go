package oauth

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
)

// OAuthLogin returns the URL that starts a sign-in through req.Method.
func (s *Service) OAuthLogin(ctx context.Context, req *dto.OAuthLoginRequest) (*dto.OAuthLoginResponse, error) {
	if err := s.deps.Policy.EnsureMethodEnabled(ctx, req.Method); err != nil {
		return nil, err
	}
	uri, err := s.deps.Flow.AuthURL(ctx, req.Method, req.Redirect)
	if err != nil {
		return nil, err
	}
	return &dto.OAuthLoginResponse{Redirect: uri}, nil
}
