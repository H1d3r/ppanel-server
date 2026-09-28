package profile

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	usermodel "github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/authn/registerpolicy"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// errUnexpectedCall fails a binding operation the email flow does not make.
var errUnexpectedCall = errors.New("unexpected call")

// bindEmailAuthRepo holds no binding yet and records the ones inserted.
type bindEmailAuthRepo struct {
	inserted []*usermodel.AuthMethods
}

var _ UserAuths = (*bindEmailAuthRepo)(nil)

func (r *bindEmailAuthRepo) FindUserAuthMethods(context.Context, int64) ([]*usermodel.AuthMethods, error) {
	return nil, errUnexpectedCall
}

func (r *bindEmailAuthRepo) FindUserAuthMethodByOpenID(context.Context, string, string) (*usermodel.AuthMethods, error) {
	return &usermodel.AuthMethods{}, gorm.ErrRecordNotFound
}

func (r *bindEmailAuthRepo) FindUserAuthMethodByUserId(context.Context, string, int64) (*usermodel.AuthMethods, error) {
	return &usermodel.AuthMethods{}, gorm.ErrRecordNotFound
}

func (r *bindEmailAuthRepo) FindUserAuthMethodByPlatform(context.Context, int64, string) (*usermodel.AuthMethods, error) {
	return nil, errUnexpectedCall
}

func (r *bindEmailAuthRepo) InsertUserAuthMethods(_ context.Context, data *usermodel.AuthMethods) error {
	r.inserted = append(r.inserted, data)
	return nil
}

func (r *bindEmailAuthRepo) UpdateUserAuthMethods(context.Context, *usermodel.AuthMethods) error {
	return errUnexpectedCall
}

func (r *bindEmailAuthRepo) DeleteUserAuthMethods(context.Context, int64, string) error {
	return errUnexpectedCall
}

// newBindEmailService returns the service, its bindings and Redis, and the
// context of the signed-in account 7.
func newBindEmailService(t *testing.T) (*Service, *bindEmailAuthRepo, *redis.Client, context.Context) {
	t.Helper()
	rds := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	t.Cleanup(func() { _ = rds.Close() })
	repo := &bindEmailAuthRepo{}
	ctx := usermodel.NewContext(context.Background(), &usermodel.User{Id: 7})
	return NewService(Deps{
		UserAuth:     repo,
		Redis:        rds,
		Policy:       registerpolicy.New(registerpolicy.Deps{Config: func() registerpolicy.Snapshot { return registerpolicy.Snapshot{EmailEnabled: true} }}),
		EmailDomains: func() (string, bool) { return "", false },
	}), repo, rds, ctx
}

// The bound email becomes a login and password-reset identifier, so the
// caller must prove control of the address; a session alone is not enough.
func TestUpdateBindEmailRequiresCodeSentToNewAddress(t *testing.T) {
	svc, repo, rds, ctx := newBindEmailService(t)
	key := verification.EmailCodeKey(auth.Register, "new@example.com")
	if err := verification.SaveVerificationCode(context.Background(), rds, key, "123456", time.Minute); err != nil {
		t.Fatal(err)
	}

	err := svc.UpdateBindEmail(ctx, &dto.UpdateBindEmailRequest{Email: "new@example.com", Code: "000000"})
	var codeErr *xerr.CodeError
	if !errors.As(err, &codeErr) || codeErr.GetErrCode() != xerr.VerifyCodeError || len(repo.inserted) != 0 {
		t.Fatalf("wrong code: error = %v, inserted = %d, want VerifyCodeError and no binding", err, len(repo.inserted))
	}

	if err := svc.UpdateBindEmail(ctx, &dto.UpdateBindEmailRequest{Email: "new@example.com", Code: "123456"}); err != nil {
		t.Fatalf("correct code: error = %v", err)
	}
	if len(repo.inserted) != 1 || !repo.inserted[0].Verified || repo.inserted[0].AuthIdentifier != "new@example.com" {
		t.Fatalf("inserted = %+v, want one verified binding for new@example.com", repo.inserted)
	}

	// The code is single use.
	if err := svc.UpdateBindEmail(ctx, &dto.UpdateBindEmailRequest{Email: "new@example.com", Code: "123456"}); err == nil {
		t.Fatal("a consumed code bound the address again")
	}
}
