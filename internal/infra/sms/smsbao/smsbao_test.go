package smsbao

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func smsbaoAPI(t *testing.T, body string) (*Client, *url.URL) {
	t.Helper()
	var received url.URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		received = *r.URL
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	client := NewClient(Config{Access: "panel", Secret: "s3cret"}, server.Client())
	client.baseURL = server.URL
	return client, &received
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

// Mainland numbers use the domestic endpoint without a prefix; every other
// number the international one with +<area>. The password travels as its
// MD5.
func TestSendTextBuildsTheRequestPerRegion(t *testing.T) {
	for _, tt := range []struct {
		area, path, number string
	}{
		{"86", "/sms", "13800000000"},
		{"1", "/wsms", "+113800000000"},
	} {
		client, received := smsbaoAPI(t, "0")
		if err := client.SendText(context.Background(), tt.area, "13800000000", "验证码 123456"); err != nil {
			t.Fatalf("area %s: SendText: %v", tt.area, err)
		}
		query := received.Query()
		if received.Path != tt.path || query.Get("u") != "panel" || query.Get("p") != md5Hex("s3cret") ||
			query.Get("m") != tt.number || query.Get("c") != "验证码 123456" {
			t.Fatalf("area %s: request = %s?%s", tt.area, received.Path, received.RawQuery)
		}
	}
}

func TestSendTextParsesTheStatusCode(t *testing.T) {
	for body, want := range map[string]string{
		"0":   "",
		"0\n": "",
		"30":  "Password error",
		"41":  "Insufficient balance",
		"51":  "Mobile number is incorrect",
		"99":  "unknown error",
	} {
		client, _ := smsbaoAPI(t, body)
		err := client.SendText(context.Background(), "86", "13800000000", "hi")
		if want == "" {
			if err != nil {
				t.Fatalf("body %q: error = %v, want success", body, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("body %q: error = %v, want %q", body, err, want)
		}
	}
}

func TestSendTextHonoursContext(t *testing.T) {
	client, received := smsbaoAPI(t, "0")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.SendText(ctx, "86", "13800000000", "hi"); err == nil {
		t.Fatal("a cancelled send reported success")
	}
	if received.Path != "" {
		t.Fatal("a cancelled send reached the provider")
	}
}
