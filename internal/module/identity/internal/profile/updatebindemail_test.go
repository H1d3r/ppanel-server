package profile

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	usermodel "github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/authn/registerpolicy"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type bindEmailAuthRepo struct {
	repository.UserAuthRepo
	inserted []*usermodel.AuthMethods
}

func (r *bindEmailAuthRepo) FindUserAuthMethodByUserId(context.Context, string, int64) (*usermodel.AuthMethods, error) {
	return &usermodel.AuthMethods{}, gorm.ErrRecordNotFound
}

func (r *bindEmailAuthRepo) FindUserAuthMethodByOpenID(context.Context, string, string) (*usermodel.AuthMethods, error) {
	return &usermodel.AuthMethods{}, gorm.ErrRecordNotFound
}

func (r *bindEmailAuthRepo) InsertUserAuthMethods(_ context.Context, data *usermodel.AuthMethods) error {
	r.inserted = append(r.inserted, data)
	return nil
}

func newBindEmailLogic(t *testing.T) (*UpdateBindEmailLogic, *bindEmailAuthRepo, *redis.Client) {
	t.Helper()
	rds := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rds.Close() })
	repo := &bindEmailAuthRepo{}
	ctx := usermodel.NewContext(context.Background(), &usermodel.User{Id: 7})
	return newUpdateBindEmailLogic(ctx, Deps{
		UserAuth:     repo,
		Redis:        rds,
		Policy:       registerpolicy.New(registerpolicy.Deps{Config: func() registerpolicy.Snapshot { return registerpolicy.Snapshot{EmailEnabled: true} }}),
		EmailDomains: func() (string, bool) { return "", false },
	}), repo, rds
}

// The bound email becomes a login and password-reset identifier, so the
// caller must prove control of the address; a session alone is not enough.
func TestUpdateBindEmailRequiresCodeSentToNewAddress(t *testing.T) {
	logic, repo, rds := newBindEmailLogic(t)
	key := verification.EmailCodeKey(auth.Register, "new@example.com")
	if err := verification.SaveVerificationCode(context.Background(), rds, key, "123456", time.Minute); err != nil {
		t.Fatal(err)
	}

	err := logic.UpdateBindEmail(&dto.UpdateBindEmailRequest{Email: "new@example.com", Code: "000000"})
	var codeErr *xerr.CodeError
	if !errors.As(err, &codeErr) || codeErr.GetErrCode() != xerr.VerifyCodeError || len(repo.inserted) != 0 {
		t.Fatalf("wrong code: error = %v, inserted = %d, want VerifyCodeError and no binding", err, len(repo.inserted))
	}

	if err := logic.UpdateBindEmail(&dto.UpdateBindEmailRequest{Email: "new@example.com", Code: "123456"}); err != nil {
		t.Fatalf("correct code: error = %v", err)
	}
	if len(repo.inserted) != 1 || !repo.inserted[0].Verified || repo.inserted[0].AuthIdentifier != "new@example.com" {
		t.Fatalf("inserted = %+v, want one verified binding for new@example.com", repo.inserted)
	}

	// The code is single use.
	if err := logic.UpdateBindEmail(&dto.UpdateBindEmailRequest{Email: "new@example.com", Code: "123456"}); err == nil {
		t.Fatal("a consumed code bound the address again")
	}
}
