package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/identitytest"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/timeutil"
	"gorm.io/gorm"
)

// The facade serves the entry points from the module's own repositories:
// the bootstrap's administrator and auth-method reads, the server start's
// check and fix-up, the device socket's presence and the session lookups.
func TestFacadeServesTheEntryPoints(t *testing.T) {
	logtest.Discard(t)
	env := identitytest.New(t)
	svc := New(Deps{
		Users: env.Store.User(), UserAuths: env.Store.UserAuth(), Devices: env.Store.UserDevice(),
		Cache: env.Store.UserCache(), Logs: env.Store.Log(), Auths: env.Store.Auth(), Store: env.Store, Redis: env.Redis,
	})
	ctx := context.Background()

	if created, err := svc.CreateInitialAdministrator(ctx, "admin@example.com", "first-secret"); err != nil || !created {
		t.Fatalf("CreateInitialAdministrator() = %t, %v", created, err)
	}
	admins, err := svc.FindAdministratorsWithPassword(ctx, "first-secret")
	if err != nil || len(admins) != 1 {
		t.Fatalf("FindAdministratorsWithPassword() = %+v, %v, want the seeded administrator", admins, err)
	}
	if err := svc.ValidateEmailIdentities(ctx); err != nil {
		t.Fatalf("ValidateEmailIdentities() = %v", err)
	}
	if err := svc.NormalizePhoneNumbers(ctx); err != nil {
		t.Fatalf("NormalizePhoneNumbers() = %v", err)
	}

	admin := admins[0]
	device := &user.Device{UserId: admin.Id, Identifier: "admin-phone", Ip: "192.0.2.1"}
	if err := env.DB.Create(device).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkDeviceOnline(ctx, device.Identifier); err != nil {
		t.Fatalf("MarkDeviceOnline() = %v", err)
	}
	if current, err := svc.FindDeviceForAuth(ctx, device.Id); err != nil || current.UserId != admin.Id || !current.Enabled {
		t.Fatalf("FindDeviceForAuth() = %+v, %v, want the administrator's enabled device", current, err)
	}
	if err := svc.MarkDeviceOffline(ctx, admin.Id, device.Identifier, timeutil.Now()); err != nil {
		t.Fatalf("MarkDeviceOffline() = %v", err)
	}
	var records int64
	if err := env.DB.Model(&user.DeviceOnlineRecord{}).Where("identifier = ?", device.Identifier).Count(&records).Error; err != nil || records != 1 {
		t.Fatalf("online records = %d, %v, want the connection recorded", records, err)
	}
	if found, err := svc.FindUser(ctx, admin.Id); err != nil || found.IsAdmin == nil || !*found.IsAdmin {
		t.Fatalf("FindUser() = %+v, %v, want the administrator", found, err)
	}

	env.EnableMethod(t, "email", `{"platform":"smtp"}`)
	if method, err := svc.FindLoginMethod(ctx, "email"); err != nil || method.Config != `{"platform":"smtp"}` {
		t.Fatalf("FindLoginMethod(email) = %+v, %v, want the stored configuration", method, err)
	}
	if _, err := svc.FindLoginMethod(ctx, "telegram"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("FindLoginMethod(telegram) error = %v, want gorm.ErrRecordNotFound", err)
	}
}
