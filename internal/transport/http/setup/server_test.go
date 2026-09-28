package setup

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server/render"
	"github.com/perfect-panel/server/pkg/logger/logtest"
)

type failingCloser struct{ err error }

func (c failingCloser) Close() error { return c.err }

// The setup handlers used to drop the error of closing their connection pool.
func TestCloseDatabaseLogsFailures(t *testing.T) {
	logs := logtest.NewCollector(t)

	closeDatabase(failingCloser{err: errors.New("connection reset")})

	if out := logs.String(); !strings.Contains(out, "close database connection failed") || !strings.Contains(out, "connection reset") {
		t.Fatalf("log = %s, want the close failure", out)
	}

	logs.Reset()
	closeDatabase(failingCloser{})
	if out := logs.String(); out != "" {
		t.Fatalf("a clean close logged %s", out)
	}
}

func TestNewConfigServer_rendersInitAndRedirectsUnknownRoutes(t *testing.T) {
	// Given
	engine := newConfigServer()
	templates := template.Must(template.ParseFS(templateFS, "templates/*.html"))

	initRequest := engine.NewContext()
	initRequest.HTMLRender = render.HTMLProduction{Template: templates}
	initRequest.Request.SetRequestURI("/init")
	initRequest.Request.Header.SetMethod(http.MethodGet)

	unknownRequest := engine.NewContext()
	unknownRequest.Request.SetRequestURI("/unknown")
	unknownRequest.Request.Header.SetMethod(http.MethodGet)

	// When
	engine.ServeHTTP(context.Background(), initRequest)
	engine.ServeHTTP(context.Background(), unknownRequest)

	// Then
	if status := initRequest.Response.StatusCode(); status != http.StatusOK {
		t.Fatalf("expected init status %d, got %d", http.StatusOK, status)
	}
	if len(initRequest.Response.Body()) == 0 {
		t.Fatal("expected init HTML response body")
	}
	if status := unknownRequest.Response.StatusCode(); status != http.StatusFound {
		t.Fatalf("expected redirect status %d, got %d", http.StatusFound, status)
	}
	if location := string(unknownRequest.Response.Header.Peek("Location")); location != "/init" {
		t.Fatalf("expected redirect location %q, got %q", "/init", location)
	}
}

// A new database stores times in the application's zone, the configuration
// file's AppLocation; the parameters are written out so a later AppLocation
// change cannot reinterpret them.
func TestBuildDatabaseConfigUsesTheAppLocation(t *testing.T) {
	for driver, want := range map[string]string{
		"mysql":    "loc=Europe%2FParis",
		"postgres": "TimeZone=Europe/Paris",
	} {
		cfg, err := buildDatabaseConfig(driver, "127.0.0.1", "3306", "ppanel", "root", "secret", "Europe/Paris")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cfg.Config, want) {
			t.Errorf("%s parameters = %q, want %s", driver, cfg.Config, want)
		}
	}
}
