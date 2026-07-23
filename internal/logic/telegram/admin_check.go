package telegram

import (
	"context"
	"strconv"

	"github.com/perfect-panel/server/internal/model/entity/user"
	"github.com/perfect-panel/server/internal/svc"
)

// resolveTelegramUser resolves a Telegram chat ID to a user, regardless of admin status.
// Used when sending messages to users via their bound Telegram.
func resolveTelegramUser(ctx context.Context, svcCtx *svc.ServiceContext, chatID int64) (*user.User, bool) {
	auth, err := svcCtx.Store.UserAuth().FindUserAuthMethodByOpenID(ctx, "telegram", strconv.FormatInt(chatID, 10))
	if err != nil {
		return nil, false
	}
	u, err := svcCtx.Store.User().FindOne(ctx, auth.UserId)
	if err != nil {
		return nil, false
	}
	return u, true
}
