package emailLogic

import (
	"testing"

	"github.com/perfect-panel/server/queue/types"
)

func TestEmailLogContentRedactsVerificationCode(t *testing.T) {
	content := map[string]interface{}{"Code": "123456", "SiteName": "Example"}

	redacted := emailLogContent(types.EmailTypeVerify, content)
	if redacted["redacted"] != true {
		t.Fatalf("verification log content = %#v", redacted)
	}
	if _, ok := redacted["Code"]; ok {
		t.Fatalf("verification log contains code: %#v", redacted)
	}
	if content["Code"] != "123456" {
		t.Fatalf("rendering content was mutated: %#v", content)
	}
}

func TestEmailLogContentPreservesNonVerificationContent(t *testing.T) {
	content := map[string]interface{}{"message": "maintenance"}

	if got := emailLogContent(types.EmailTypeMaintenance, content); got["message"] != "maintenance" {
		t.Fatalf("non-verification log content = %#v", got)
	}
}
