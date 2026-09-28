package twilio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// redirect sends every request to the test server instead of
// api.twilio.com, where the SDK addresses it.
type redirect struct{ target *url.URL }

func (r redirect) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.target.Scheme, r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

type twilioRequest struct {
	method, path, user, password string
	form                         url.Values
}

func twilioAPI(t *testing.T, status int, body string) (*Client, *twilioRequest) {
	t.Helper()
	received := &twilioRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.method, received.path = r.Method, r.URL.Path
		received.user, received.password, _ = r.BasicAuth()
		data, _ := io.ReadAll(r.Body)
		received.form, _ = url.ParseQuery(string(data))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	client := NewClient(Config{Access: "AC123", Secret: "authtoken", PhoneNumber: "+15550001111"},
		&http.Client{Transport: redirect{target: target}})
	return client, received
}

func TestSendTextCreatesAMessage(t *testing.T) {
	client, received := twilioAPI(t, http.StatusCreated, `{"sid":"SM1","status":"queued","error_code":null,"error_message":null}`)

	if err := client.SendText(context.Background(), "44", "7700900123", "Your code is 123456"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if received.method != http.MethodPost || received.path != "/2010-04-01/Accounts/AC123/Messages.json" {
		t.Fatalf("request = %s %s", received.method, received.path)
	}
	if received.user != "AC123" || received.password != "authtoken" {
		t.Fatalf("credentials = %s:%s, want the account's", received.user, received.password)
	}
	if received.form.Get("To") != "+447700900123" || received.form.Get("From") != "+15550001111" || received.form.Get("Body") != "Your code is 123456" {
		t.Fatalf("form = %v", received.form)
	}
}

func TestSendTextReportsTwilioErrors(t *testing.T) {
	for name, tt := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"rejected":         {http.StatusBadRequest, `{"code":21211,"message":"Invalid 'To' Phone Number","more_info":"https://www.twilio.com/docs/errors/21211","status":400}`, "Invalid 'To' Phone Number"},
		"accepted, failed": {http.StatusCreated, `{"sid":"SM1","error_code":30007,"error_message":"Carrier violation"}`, "Carrier violation"},
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := twilioAPI(t, tt.status, tt.body)
			err := client.SendText(context.Background(), "44", "7700900123", "hi")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

// The SDK takes no context; the client carries it to the request.
func TestSendTextHonoursContext(t *testing.T) {
	client, received := twilioAPI(t, http.StatusCreated, `{"sid":"SM1"}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.SendText(ctx, "44", "7700900123", "hi"); err == nil {
		t.Fatal("a cancelled send reported success")
	}
	if received.path != "" {
		t.Fatal("a cancelled send reached Twilio")
	}
}
