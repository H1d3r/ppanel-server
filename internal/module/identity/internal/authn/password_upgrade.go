package authn

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
)

// upgradePasswordAfterLogin rehashes a password stored with a legacy
// algorithm once its owner proved it. The rehash is best-effort: a failure
// is logged and the sign-in goes on.
func upgradePasswordAfterLogin(ctx context.Context, users repository.UserRepo, userInfo *user.User, plainPassword string) {
	if userInfo == nil || userInfo.Id == 0 || plainPassword == "" {
		return
	}
	if !password.PasswordNeedsRehash(userInfo.Algo, userInfo.Password) {
		return
	}

	nextHash := password.EncodePassWord(plainPassword)
	updated, err := users.UpgradePasswordHash(ctx, userInfo.Id, userInfo.Password, nextHash, password.PasswordAlgoArgon2id, "")
	if err != nil {
		logger.WithContext(ctx).Errorw("failed to upgrade password hash",
			logger.Field("user_id", userInfo.Id),
			logger.Field("error", err.Error()),
		)
		return
	}
	if !updated {
		return
	}
	userInfo.Password = nextHash
	userInfo.Algo = password.PasswordAlgoArgon2id
	userInfo.Salt = ""
}
