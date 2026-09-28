package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
)

// BindTelegramChat records chatID as the user's verified Telegram binding.
// It completes BindTelegram: the bot redeems the deep link's token, checks
// that neither the chat nor the account is bound yet and asks identity to
// store the binding.
func (s *Service) BindTelegramChat(ctx context.Context, userID int64, chatID string) error {
	now := timeutil.Now()
	if err := s.deps.UserAuth.InsertUserAuthMethods(ctx, &user.AuthMethods{
		UserId:         userID,
		AuthType:       "telegram",
		AuthIdentifier: chatID,
		Verified:       true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseInsertError, "bind telegram chat of user %d", userID)
	}
	// The binding is stored; a stale cache entry only delays its visibility.
	if err := s.deps.UserCache.ClearUserCache(ctx, &user.User{Id: userID}); err != nil {
		logger.WithContext(ctx).Errorw("[Telegram] refresh user cache after bind failed",
			logger.Field("error", err.Error()), logger.Field("user_id", userID))
	}
	return nil
}
