package profile

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
)

// BindOAuth returns the URL that starts binding a req.Method identity to the
// calling account.
func (s *Service) BindOAuth(ctx context.Context, req *dto.BindOAuthRequest) (*dto.BindOAuthResponse, error) {
	if err := s.deps.Policy.EnsureMethodEnabled(ctx, req.Method); err != nil {
		return nil, err
	}
	uri, err := s.deps.OAuth.AuthURL(ctx, req.Method, req.Redirect)
	if err != nil {
		return nil, err
	}
	return &dto.BindOAuthResponse{Redirect: uri}, nil
}
