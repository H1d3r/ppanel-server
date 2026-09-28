package sms

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/perfect-panel/server/pkg/logger/logtest"
)

// Building a sender runs on every SMS send; the provider config it receives
// holds access keys, auth tokens and passwords, none of which may reach any
// log sink.
func TestNewSenderDoesNotLogProviderCredentials(t *testing.T) {
	const secret = "sentinel-provider-secret"
	var stdlog bytes.Buffer
	log.SetOutput(&stdlog)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	collector := logtest.NewCollector(t)

	for _, platform := range []string{AlibabaCloud.String(), Smsbao.String(), Abosend.String(), Twilio.String()} {
		config := fmt.Sprintf(`{"access":"access-id","secret":%q,"endpoint":"dysmsapi.aliyuncs.com","template":"{{.code}}"}`, secret)
		if _, err := NewSender(platform, config); err != nil {
			t.Fatalf("NewSender(%s): %v", platform, err)
		}
	}

	if strings.Contains(stdlog.String(), secret) || strings.Contains(collector.String(), secret) {
		t.Fatalf("provider secret leaked into logs:\nlog: %s\nlogger: %s", stdlog.String(), collector.String())
	}
}
