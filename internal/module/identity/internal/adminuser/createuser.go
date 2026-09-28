package adminuser

import (
	"context"
	"errors"
	"fmt"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/account"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

// CreateUser creates an account on the administrator's behalf. Its phone
// number is stored in E.164 like every self-service one, so the account signs
// in and resets by phone; an identifier another account holds is refused.
func (s *Service) CreateUser(ctx context.Context, req *dto.CreateUserRequest) error {
	referCode := req.ReferCode
	if referCode == "" {
		// timestamp replaces user id
		referCode = user.GenerateInviteCode(timeutil.Now().UnixMicro())
	}
	plain := req.Password
	if plain == "" {
		plain = req.Email
	}
	newUser := &user.User{
		Password:           password.EncodePassWord(plain),
		Algo:               password.PasswordAlgoArgon2id,
		ReferralPercentage: req.ReferralPercentage,
		OnlyFirstPurchase:  &req.OnlyFirstPurchase,
		ReferCode:          referCode,
		IsAdmin:            &req.IsAdmin,
	}

	var identities []user.AuthMethods
	if req.TelephoneAreaCode != "" && req.Telephone != "" {
		phone, err := identifier.FormatToE164(req.TelephoneAreaCode, req.Telephone)
		if err != nil {
			return xerr.Wrapf(err, xerr.TelephoneError, "invalid phone number")
		}
		if err := s.ensureIdentityFree(ctx, identifier.Mobile, phone, xerr.TelephoneExist); err != nil {
			return err
		}
		identities = append(identities, user.AuthMethods{AuthType: identifier.Mobile, AuthIdentifier: phone})
	}
	if req.Email != "" {
		if err := s.ensureIdentityFree(ctx, identifier.Email, req.Email, xerr.EmailExist); err != nil {
			return err
		}
		identities = append(identities, user.AuthMethods{AuthType: identifier.Email, AuthIdentifier: req.Email})
	}

	if req.RefererUser != "" {
		referer, err := s.deps.Users.FindOneByEmail(ctx, req.RefererUser)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("referer user not found: %w", xerr.NewErrCode(xerr.UserNotExist))
			}
			return xerr.Wrapf(err, xerr.DatabaseQueryError, "find referer user")
		}
		newUser.RefererId = referer.Id
	}

	// Two sequential domain transactions replace the old cross-domain one:
	// the identity transaction creates the account (and its zero wallet
	// row); the billing transaction credits the initial money. A failure
	// between them leaves an uncredited account the admin can adjust — the
	// same partial-failure surface the flows will have as services.
	if err := s.deps.Store.InIdentityTx(ctx, func(tx repository.IdentityStore) error {
		return account.Create(ctx, tx, account.New{User: newUser, Identities: identities})
	}); err != nil {
		return err
	}
	if req.Balance == 0 && req.Commission == 0 && req.GiftAmount == 0 {
		return nil
	}
	return s.deps.Store.InBillingTx(ctx, func(store repository.BillingStore) error {
		w, err := store.Wallet().FindOneForUpdate(ctx, newUser.Id)
		if err != nil {
			return xerr.Wrapf(err, xerr.DatabaseQueryError, "load new user wallet")
		}
		w.Balance = req.Balance
		w.GiftAmount = req.GiftAmount
		w.Commission = req.Commission
		if err := store.Wallet().UpdateBalanceFields(ctx, w); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "credit new user wallet")
		}
		if err := store.Wallet().UpdateCommission(ctx, w); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "credit new user commission")
		}
		return nil
	})
}

// ensureIdentityFree refuses an identifier another account holds with the
// taken code; a failed lookup is a database error, not a free identifier.
func (s *Service) ensureIdentityFree(ctx context.Context, authType, authIdentifier string, taken uint32) error {
	_, err := s.deps.UserAuths.FindUserAuthMethodByOpenID(ctx, authType, authIdentifier)
	switch {
	case err == nil:
		return fmt.Errorf("the %s identifier is bound to an account: %w", authType, xerr.NewErrCode(taken))
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil
	default:
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find %s identity", authType)
	}
}
