package usersession

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/perfect-panel/server/internal/auth/devicesession"
	"github.com/perfect-panel/server/internal/auth/token"
	"github.com/redis/go-redis/v9"
)

const testSecret = "session-test-secret"

func newTestClient(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { _ = client.Close() })
	return server, client
}

func TestIssuedSessionValidatesUntilEnded(t *testing.T) {
	server, client := newTestClient(t)
	ctx := context.Background()

	signed, err := Issue(ctx, client, testSecret, 3600, Grant{UserID: 7, LoginType: "email"})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	claims, err := Validate(ctx, client, testSecret, signed)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if claims.UserID != 7 || claims.LoginType != "email" || claims.SessionID == "" || claims.DeviceID != 0 {
		t.Fatalf("claims = %+v", claims)
	}
	if ttl := server.TTL(SessionKey(claims.SessionID)); ttl <= 0 || ttl > time.Hour {
		t.Fatalf("session record TTL = %v, want the token lifetime", ttl)
	}

	if err := End(ctx, client, claims.SessionID); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(ctx, client, testSecret, signed); !errors.Is(err, ErrEnded) {
		t.Fatalf("ended session: error = %v, want ErrEnded", err)
	}
}

func TestValidateRejectsWhatIsNotALiveSession(t *testing.T) {
	_, client := newTestClient(t)
	ctx := context.Background()

	if _, err := Validate(ctx, client, testSecret, "not-a-token"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("garbage token: error = %v, want ErrInvalidToken", err)
	}
	signed, err := Issue(ctx, client, testSecret, 3600, Grant{UserID: 7})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(ctx, client, "another-secret", signed); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("foreign secret: error = %v, want ErrInvalidToken", err)
	}
	// Order event tickets share the secret but carry no session.
	ticket, err := token.NewJwtToken(testSecret, time.Now().Unix(), 3600, token.WithOption("OrderNo", "1"), token.WithOption(UserIDClaim, int64(7)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(ctx, client, testSecret, ticket); !errors.Is(err, ErrNotSession) {
		t.Fatalf("ticket: error = %v, want ErrNotSession", err)
	}

	if err := Revoke(ctx, client, 7); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(ctx, client, testSecret, signed); !errors.Is(err, ErrRevoked) {
		t.Fatalf("revoked session: error = %v, want ErrRevoked", err)
	}
}

func TestDeviceSessionEndsWithItsDevice(t *testing.T) {
	_, client := newTestClient(t)
	ctx := context.Background()

	signed, err := Issue(ctx, client, testSecret, 3600, Grant{UserID: 7, LoginType: "device", DeviceID: 9})
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	claims, err := Validate(ctx, client, testSecret, signed)
	if err != nil || claims.DeviceID != 9 || claims.LoginType != "device" {
		t.Fatalf("Validate() = %+v, %v", claims, err)
	}
	other, err := Issue(ctx, client, testSecret, 3600, Grant{UserID: 7, DeviceID: 10})
	if err != nil {
		t.Fatal(err)
	}

	if err := devicesession.Revoke(ctx, client, 9); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(ctx, client, testSecret, signed); !errors.Is(err, ErrDeviceRevoked) {
		t.Fatalf("revoked device: error = %v, want ErrDeviceRevoked", err)
	}
	if _, err := Validate(ctx, client, testSecret, other); err != nil {
		t.Fatalf("another device's session ended too: %v", err)
	}
}

// A device session issued before sessions were bound to devices carries no
// binding and must be renewed.
func TestValidateRejectsUnboundDeviceSession(t *testing.T) {
	_, client := newTestClient(t)
	ctx := context.Background()
	epoch, err := AcquireEpoch(ctx, client, 7)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := token.NewJwtToken(testSecret, time.Now().Unix(), 3600,
		token.WithOption(UserIDClaim, int64(7)), token.WithOption(SessionIDClaim, "legacy"),
		token.WithOption(LoginTypeClaim, "device"), token.WithOption(EpochClaim, epoch))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Set(ctx, SessionKey("legacy"), 7, 0).Err(); err != nil {
		t.Fatal(err)
	}
	if _, err := Validate(ctx, client, testSecret, legacy); !errors.Is(err, ErrDeviceSession) {
		t.Fatalf("legacy device session: error = %v, want ErrDeviceSession", err)
	}
}

func TestIssueAndValidateFailClosedWithoutStore(t *testing.T) {
	var client *redis.Client
	if _, err := Issue(context.Background(), client, testSecret, 3600, Grant{UserID: 7}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Issue() error = %v, want ErrUnavailable", err)
	}
	if _, err := Validate(context.Background(), client, testSecret, "token"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Validate() error = %v, want ErrUnavailable", err)
	}
	_, live := newTestClient(t)
	if _, err := Issue(context.Background(), live, testSecret, 0, Grant{UserID: 7}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Issue() without a lifetime: error = %v, want ErrUnavailable", err)
	}
}
