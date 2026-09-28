package checkout

import (
	"context"
	"errors"
	"testing"
	"time"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	userEntity "github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/subscription/entity/usersub"
	"github.com/perfect-panel/server/pkg/xerr"
)

type ownershipUserSubs struct {
	UserSubscriptionReader
	subscribe *usersub.SubscribeDetails
}

func (r ownershipUserSubs) FindOneUserSubscribe(_ context.Context, _ int64) (*usersub.SubscribeDetails, error) {
	return r.subscribe, nil
}

func ownerContext(id int64) context.Context {
	return userEntity.NewContext(context.Background(), &userEntity.User{Id: id})
}

func TestRenewalRejectsSubscriptionOwnedByAnotherUser(t *testing.T) {
	svc := NewService(Deps{UserSubs: ownershipUserSubs{subscribe: &usersub.SubscribeDetails{Id: 22, UserId: 33}}})

	_, err := svc.Renewal(ownerContext(11), &dto.RenewalOrderRequest{UserSubscribeID: 22})
	assertCode(t, err, xerr.InvalidAccess)
}

func TestRenewalRejectsDeductedSubscription(t *testing.T) {
	svc := NewService(Deps{UserSubs: ownershipUserSubs{subscribe: &usersub.SubscribeDetails{Id: 22, UserId: 11, Status: usersub.SubscribeStatusDeducted}}})

	_, err := svc.Renewal(ownerContext(11), &dto.RenewalOrderRequest{UserSubscribeID: 22})
	assertCode(t, err, xerr.SubscribeNotAvailable)
}

func TestResetTrafficRejectsSubscriptionOwnedByAnotherUser(t *testing.T) {
	svc := NewService(Deps{UserSubs: ownershipUserSubs{subscribe: &usersub.SubscribeDetails{Id: 22, UserId: 33}}})

	_, err := svc.ResetTraffic(ownerContext(11), &dto.ResetTrafficOrderRequest{UserSubscribeID: 22})
	assertCode(t, err, xerr.InvalidAccess)
}

func TestResetTrafficRejectsExpiredSubscription(t *testing.T) {
	svc := NewService(Deps{UserSubs: ownershipUserSubs{subscribe: &usersub.SubscribeDetails{
		Id: 22, UserId: 11, ExpireTime: time.Now().Add(-time.Minute),
	}}})

	_, err := svc.ResetTraffic(ownerContext(11), &dto.ResetTrafficOrderRequest{UserSubscribeID: 22})
	assertCode(t, err, xerr.SubscribeNotAvailable)
}

func TestLocalCheckoutRejectsProviderManagedSubscription(t *testing.T) {
	svc := NewService(Deps{UserSubs: ownershipUserSubs{subscribe: &usersub.SubscribeDetails{Id: 22, UserId: 11, EntitlementSource: "apple"}}})
	_, err := svc.Renewal(ownerContext(11), &dto.RenewalOrderRequest{UserSubscribeID: 22, Quantity: 1})
	if !errors.Is(err, usersub.ErrProviderManaged) {
		t.Fatalf("local renewal accepted: %v", err)
	}
	_, err = svc.ResetTraffic(ownerContext(11), &dto.ResetTrafficOrderRequest{UserSubscribeID: 22})
	if !errors.Is(err, usersub.ErrProviderManaged) {
		t.Fatalf("local reset checkout accepted: %v", err)
	}
}
