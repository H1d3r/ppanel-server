package adminuser

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/auth/password"
	walletEntity "github.com/perfect-panel/server/internal/module/billing/entity/wallet"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/identitytest"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/xerr"
)

// wallets is the billing wallet table the admin flows read. Moving money is
// billing's; these tests leave the wallets as they are.
type wallets struct {
	repository.WalletRepo
	rows map[int64]*walletEntity.Wallet
}

func (w *wallets) FindOneForUpdate(_ context.Context, userID int64) (*walletEntity.Wallet, error) {
	row, ok := w.rows[userID]
	if !ok {
		row = &walletEntity.Wallet{UserId: userID}
		w.rows[userID] = row
	}
	copied := *row
	return &copied, nil
}

func (w *wallets) FindWallet(_ context.Context, userID int64) (*walletEntity.Wallet, error) {
	return w.rows[userID], nil
}

func (w *wallets) FindWalletsByUserIds(_ context.Context, ids []int64) (map[int64]*walletEntity.Wallet, error) {
	found := make(map[int64]*walletEntity.Wallet)
	for _, id := range ids {
		if row, ok := w.rows[id]; ok {
			found[id] = row
		}
	}
	return found, nil
}

// adminStore runs identity transactions on the real store and billing ones
// on the wallet table above, with the real audit log.
type adminStore struct {
	*repository.GormStore
	wallets *wallets
}

func (s adminStore) InBillingTx(ctx context.Context, fn func(repository.BillingStore) error) error {
	return fn(billingView{wallets: s.wallets, logs: s.Log()})
}

type billingView struct {
	repository.BillingStore
	wallets *wallets
	logs    repository.LogRepo
}

func (v billingView) Wallet() repository.WalletRepo { return v.wallets }
func (v billingView) Log() repository.LogRepo       { return v.logs }

type fixture struct {
	*identitytest.Env
	svc     *Service
	wallets *wallets
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	logtest.Discard(t)
	env := identitytest.New(t)
	w := &wallets{rows: map[int64]*walletEntity.Wallet{}}
	svc := NewService(Deps{
		Users: env.Store.User(), UserAuths: env.Store.UserAuth(), Devices: env.Store.UserDevice(),
		Cache: env.Store.UserCache(), Logs: env.Store.Log(), Wallet: w,
		Store: adminStore{GormStore: env.Store, wallets: w}, Redis: env.Redis,
	})
	return &fixture{Env: env, svc: svc, wallets: w}
}

func (f *fixture) account(t *testing.T, authType, identifier string) *user.User {
	t.Helper()
	enabled := true
	u := &user.User{Enable: &enabled, ReferCode: "REF-" + identifier}
	if err := f.DB.Create(u).Error; err != nil {
		t.Fatal(err)
	}
	if identifier != "" {
		if err := f.DB.Create(&user.AuthMethods{UserId: u.Id, AuthType: authType, AuthIdentifier: identifier, Verified: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return u
}

func assertCode(t *testing.T, err error, want uint32) {
	t.Helper()
	if got := xerr.CodeOf(err); err == nil || got != want {
		t.Fatalf("error = %v (code %d), want code %d", err, got, want)
	}
}

// An account the administrator creates keeps its phone number in E.164, the
// form sign-in and password reset look it up in; it used to be stored as
// "<area>-<number>" and never found.
func TestCreateUserStoresThePhoneNumberInE164(t *testing.T) {
	f := newFixture(t)
	err := f.svc.CreateUser(context.Background(), &dto.CreateUserRequest{
		Email: "new@example.com", Password: "password-1", TelephoneAreaCode: "86", Telephone: "13800138000",
		ReferCode: "ADMIN-MADE",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	users := f.Users(t)
	if len(users) != 1 || users[0].ReferCode != "ADMIN-MADE" || !password.MultiPasswordVerify(users[0].Algo, users[0].Salt, "password-1", users[0].Password) {
		t.Fatalf("accounts = %+v", users)
	}
	found, err := f.Store.UserAuth().FindUserAuthMethodByOpenID(context.Background(), "mobile", "+8613800138000")
	if err != nil || found.UserId != users[0].Id {
		t.Fatalf("the E.164 lookup sign-in uses = %+v, %v", found, err)
	}
	if identities := f.Identities(t, users[0].Id); len(identities) != 2 {
		t.Fatalf("identities = %+v, want the phone number and the email", identities)
	}

	list, err := f.svc.GetUserList(context.Background(), &dto.GetUserListRequest{Page: 1, Size: 10})
	if err != nil || len(list.List) != 1 {
		t.Fatalf("GetUserList = %+v, %v", list, err)
	}
	for _, method := range list.List[0].AuthMethods {
		if method.AuthType == "mobile" && method.AuthIdentifier != "+86 138 0013 8000" {
			t.Fatalf("listed number = %q, want the international form", method.AuthIdentifier)
		}
	}
}

// A number or address another account holds is refused, whatever form the
// administrator types it in.
func TestCreateUserRefusesTakenIdentifiers(t *testing.T) {
	f := newFixture(t)
	f.account(t, "mobile", "+8613800138000")
	f.account(t, "email", "taken@example.com")

	err := f.svc.CreateUser(context.Background(), &dto.CreateUserRequest{TelephoneAreaCode: "86", Telephone: "138-0013-8000"})
	assertCode(t, err, xerr.TelephoneExist)
	err = f.svc.CreateUser(context.Background(), &dto.CreateUserRequest{Email: "TAKEN@example.com"})
	assertCode(t, err, xerr.EmailExist)
	err = f.svc.CreateUser(context.Background(), &dto.CreateUserRequest{TelephoneAreaCode: "86", Telephone: "not a number"})
	assertCode(t, err, xerr.TelephoneError)
	if n := len(f.Users(t)); n != 2 {
		t.Fatalf("accounts = %d, want only the existing two", n)
	}
}

// failingAuths is an identity table that cannot be read.
type failingAuths struct{ repository.UserAuthRepo }

func (failingAuths) FindUserAuthMethodByOpenID(context.Context, string, string) (*user.AuthMethods, error) {
	return &user.AuthMethods{}, errors.New("database unavailable")
}

// A failed duplicate check is a database error, not a free identifier.
func TestCreateUserReportsAFailedDuplicateCheck(t *testing.T) {
	f := newFixture(t)
	f.svc.deps.UserAuths = failingAuths{f.Store.UserAuth()}
	err := f.svc.CreateUser(context.Background(), &dto.CreateUserRequest{TelephoneAreaCode: "86", Telephone: "13800138000"})
	assertCode(t, err, xerr.DatabaseQueryError)
	if n := len(f.Users(t)); n != 0 {
		t.Fatalf("accounts = %d, want none", n)
	}
}

// A rejected edit reports why: the transaction error used to be replaced by
// a database error.
func TestUpdateUserBasicInfoReportsValidationCodes(t *testing.T) {
	f := newFixture(t)
	target := f.account(t, "email", "owner@example.com")

	err := f.svc.UpdateUserBasicInfo(context.Background(), &dto.UpdateUserBasicInfoRequest{UserId: target.Id, Avatar: "not-an-image", Enable: true})
	assertCode(t, err, xerr.InvalidParams)

	t.Setenv("PPANEL_MODE", "demo")
	demoAdmin := f.account(t, "email", "admin@example.com")
	if demoAdmin.Id != demoAdminID {
		t.Fatalf("demo admin id = %d, want %d", demoAdmin.Id, demoAdminID)
	}
	err = f.svc.UpdateUserBasicInfo(context.Background(), &dto.UpdateUserBasicInfoRequest{UserId: demoAdmin.Id, Password: "new-password", Enable: true})
	assertCode(t, err, xerr.DemoModeRestricted)
}

func TestUpdateUserBasicInfoWritesTheProfile(t *testing.T) {
	f := newFixture(t)
	target := f.account(t, "email", "owner@example.com")

	err := f.svc.UpdateUserBasicInfo(context.Background(), &dto.UpdateUserBasicInfoRequest{
		UserId: target.Id, ReferCode: "RENAMED", Enable: true, Password: "new-password",
	})
	if err != nil {
		t.Fatalf("UpdateUserBasicInfo() error = %v", err)
	}
	var stored user.User
	if err := f.DB.First(&stored, target.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ReferCode != "RENAMED" || !password.MultiPasswordVerify(stored.Algo, stored.Salt, "new-password", stored.Password) {
		t.Fatalf("stored = %+v", stored)
	}
	if rows := f.Logs(t, log.TypeBalance, target.Id); len(rows) != 0 {
		t.Fatalf("balance audits = %d, want none for an unchanged wallet", len(rows))
	}
}

// The demo instance's administrator cannot be deleted.
func TestDeletingTheDemoAdministratorIsRefused(t *testing.T) {
	f := newFixture(t)
	f.account(t, "email", "first@example.com")
	admin := f.account(t, "email", "admin@example.com")
	t.Setenv("PPANEL_MODE", "demo")

	assertCode(t, f.svc.DeleteUser(context.Background(), &dto.GetDetailRequest{Id: admin.Id}), xerr.DemoModeRestricted)
	assertCode(t, f.svc.BatchDeleteUser(context.Background(), &dto.BatchDeleteUserRequest{Ids: []int64{1, admin.Id}}), xerr.DemoModeRestricted)
	for _, u := range f.Users(t) {
		if u.DeletedAt.Valid {
			t.Fatalf("user %d was deleted", u.Id)
		}
	}
	t.Setenv("PPANEL_MODE", "")
	if err := f.svc.DeleteUser(context.Background(), &dto.GetDetailRequest{Id: admin.Id}); err != nil {
		t.Fatalf("DeleteUser() outside demo mode: %v", err)
	}
}

// An administrator's binding is normalized like a self-service one, and a
// number that cannot be is refused.
func TestCreateUserAuthMethodNormalizesTheIdentifier(t *testing.T) {
	f := newFixture(t)
	target := f.account(t, "email", "")

	if err := f.svc.CreateUserAuthMethod(context.Background(), &dto.CreateUserAuthMethodRequest{UserId: target.Id, AuthType: "mobile", AuthIdentifier: "86-13800138000"}); err != nil {
		t.Fatalf("CreateUserAuthMethod() error = %v", err)
	}
	identities := f.Identities(t, target.Id)
	if len(identities) != 1 || identities[0].AuthIdentifier != "+8613800138000" || !identities[0].Verified {
		t.Fatalf("identities = %+v", identities)
	}
	err := f.svc.UpdateUserAuthMethod(context.Background(), &dto.UpdateUserAuthMethodRequest{UserId: target.Id, AuthType: "mobile", AuthIdentifier: "not a number"})
	assertCode(t, err, xerr.TelephoneError)
	if err := f.svc.UpdateUserAuthMethod(context.Background(), &dto.UpdateUserAuthMethodRequest{UserId: target.Id, AuthType: "mobile", AuthIdentifier: "+86 139 0013 9000"}); err != nil {
		t.Fatalf("UpdateUserAuthMethod() error = %v", err)
	}
	if identities := f.Identities(t, target.Id); identities[0].AuthIdentifier != "+8613900139000" {
		t.Fatalf("identities = %+v", identities)
	}
	err = f.svc.CreateUserAuthMethod(context.Background(), &dto.CreateUserAuthMethodRequest{UserId: target.Id, AuthType: "mobile", AuthIdentifier: "call-me"})
	assertCode(t, err, xerr.TelephoneError)
	err = f.svc.CreateUserAuthMethod(context.Background(), &dto.CreateUserAuthMethodRequest{UserId: target.Id, AuthType: "Device", AuthIdentifier: "device-1"})
	assertCode(t, err, xerr.InvalidParams)
}
