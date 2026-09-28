package oauth

import (
	"context"
	"net/http"
	"net/url"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthstate"
	"github.com/perfect-panel/server/pkg/logger"
)

type AppleLoginRedirect struct {
	StatusCode int
	Location   string
}

// AppleLoginCallback answers Apple's form post: it sends the browser to the
// redirect the state was issued for, carrying the code and state on to the
// sign-in, which redeems them. An unknown state or a redirect off the site
// host sends the browser to the site host instead.
func (s *Service) AppleLoginCallback(ctx context.Context, req *dto.AppleLoginCallbackRequest) (*AppleLoginRedirect, error) {
	fallback := s.deps.Config().SiteHost
	log := logger.WithContext(ctx)
	stored, err := oauthstate.Peek(ctx, s.deps.Redis, "apple", req.State)
	if err != nil {
		log.Errorw("get apple state code from redis failed", logger.Field("error", err.Error()), logger.Field("code", req.State))
		return appleLoginRedirect(fallback, req, http.StatusTemporaryRedirect), nil
	}
	// Never 302 off the configured site host, even if a hostile redirect
	// slipped into the state store.
	if err := oauthstate.ValidateRedirect(stored, fallback); err != nil {
		log.Errorw("stored apple redirect rejected", logger.Field("error", err.Error()), logger.Field("redirect", stored))
		return appleLoginRedirect(fallback, req, http.StatusTemporaryRedirect), nil
	}
	redirect := appleLoginRedirect(stored, req, http.StatusFound)
	log.Infow("redirect to apple login page", logger.Field("url", redirect.Location))
	return redirect, nil
}

func appleLoginRedirect(location string, req *dto.AppleLoginCallbackRequest, statusCode int) *AppleLoginRedirect {
	if statusCode == http.StatusTemporaryRedirect {
		return &AppleLoginRedirect{StatusCode: statusCode, Location: location}
	}

	parsedLocation, err := url.Parse(location)
	if err != nil {
		return &AppleLoginRedirect{StatusCode: statusCode, Location: location}
	}

	query := parsedLocation.Query()
	query.Set("method", "apple")
	query.Set("code", req.Code)
	query.Set("state", req.State)
	parsedLocation.RawQuery = query.Encode()

	return &AppleLoginRedirect{
		StatusCode: statusCode,
		Location:   parsedLocation.String(),
	}
}
