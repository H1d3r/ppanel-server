package repo

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger/logtest"
)

// Accounts the admin panel created stored their number as "<area>-<number>"
// and could not sign in by phone. The startup fix-up moves every stored
// number to E.164, leaves a number alone when its E.164 form belongs to
// another binding, and changes nothing on a second run.
func TestNormalizeMobileIdentifiersConvertsLegacyNumbers(t *testing.T) {
	logtest.Discard(t)
	db, repo := newSQLiteUserRepo(t, "normalize-mobile-identifiers")
	ctx := context.Background()

	bindings := map[string]*user.AuthMethods{
		"legacy":    {AuthType: "mobile", AuthIdentifier: "86-13800138000"},
		"no plus":   {AuthType: "mobile", AuthIdentifier: "8613900139000"},
		"conflict":  {AuthType: "mobile", AuthIdentifier: "86-13700137000"},
		"holder":    {AuthType: "mobile", AuthIdentifier: "+8613700137000"},
		"canonical": {AuthType: "mobile", AuthIdentifier: "+8613600136000"},
		"garbage":   {AuthType: "mobile", AuthIdentifier: "call-me"},
		"device":    {AuthType: "device", AuthIdentifier: "device-86-1"},
	}
	for _, binding := range bindings {
		owner := &user.User{}
		if err := db.Create(owner).Error; err != nil {
			t.Fatal(err)
		}
		binding.UserId = owner.Id
		if err := db.Create(binding).Error; err != nil {
			t.Fatal(err)
		}
	}

	result, err := repo.NormalizeMobileIdentifiers(ctx)
	if err != nil {
		t.Fatalf("NormalizeMobileIdentifiers() error = %v", err)
	}
	if want := (repository.MobileNormalization{Converted: 2, Conflicts: 1, Unparsable: 1}); result != want {
		t.Fatalf("result = %+v, want %+v", result, want)
	}
	for name, want := range map[string]string{
		"legacy":    "+8613800138000",
		"no plus":   "+8613900139000",
		"conflict":  "86-13700137000",
		"holder":    "+8613700137000",
		"canonical": "+8613600136000",
		"garbage":   "call-me",
		"device":    "device-86-1",
	} {
		var stored user.AuthMethods
		if err := db.First(&stored, bindings[name].Id).Error; err != nil {
			t.Fatal(err)
		}
		if stored.AuthIdentifier != want {
			t.Errorf("%s binding = %q, want %q", name, stored.AuthIdentifier, want)
		}
	}

	// The converted account now signs in with the number in any form.
	found, err := repo.FindUserAuthMethodByOpenID(ctx, "mobile", "86-13800138000")
	if err != nil || found.UserId != bindings["legacy"].UserId {
		t.Fatalf("lookup after conversion = %+v, %v", found, err)
	}

	again, err := repo.NormalizeMobileIdentifiers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again.Converted != 0 {
		t.Fatalf("second run converted %d bindings, want none", again.Converted)
	}
}
