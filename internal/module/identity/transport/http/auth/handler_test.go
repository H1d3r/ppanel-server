package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/perfect-panel/server/internal/module/identity"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/transport/http/middleware"
	"github.com/perfect-panel/server/pkg/logger/logtest"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/xerr"
)

// capturedService records what the handlers hand to the identity service.
type capturedService struct {
	identity.Service
	calls    int
	metadata requestmeta.Metadata
	login    dto.UserLoginRequest
	phone    dto.TelephoneLoginRequest
}

func (s *capturedService) UserLogin(ctx context.Context, req *dto.UserLoginRequest) (*dto.LoginResponse, error) {
	s.calls++
	s.login = *req
	s.metadata, _ = requestmeta.From(ctx)
	return &dto.LoginResponse{Token: "token"}, nil
}

func (s *capturedService) TelephoneLogin(ctx context.Context, req *dto.TelephoneLoginRequest) (*dto.LoginResponse, error) {
	s.calls++
	s.phone = *req
	s.metadata, _ = requestmeta.From(ctx)
	return &dto.LoginResponse{Token: "token"}, nil
}

func serve(t *testing.T, path string, register func(*server.Hertz), body string) (code uint32, clientIP string) {
	t.Helper()
	router := server.New()
	router.Use(middleware.LoggerMiddleware())
	register(router)
	c := router.NewContext()
	c.Request.Header.SetMethod(http.MethodPost)
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("User-Agent", "Client-UA/1.0")
	// A client-supplied header is not the client address.
	c.Request.Header.Set("X-Original-Forwarded-For", "198.51.100.66")
	c.Request.SetRequestURI(path)
	c.Request.SetBodyString(body)
	router.ServeHTTP(context.Background(), c)
	var envelope struct {
		Code uint32 `json:"code"`
	}
	if err := json.Unmarshal(c.Response.Body(), &envelope); err != nil {
		t.Fatalf("response %q: %v", c.Response.Body(), err)
	}
	return envelope.Code, c.ClientIP()
}

// The flows get the client address and user agent from the request
// metadata; the request body and headers cannot supply them.
func TestLoginHandlersPassTheRequestMetadata(t *testing.T) {
	logtest.Discard(t)
	for name, tc := range map[string]struct {
		path     string
		register func(*server.Hertz, identity.Service)
		body     string
	}{
		"email": {"/v1/auth/login", func(r *server.Hertz, s identity.Service) {
			r.POST("/v1/auth/login", UserLoginHandler(s))
		}, `{"email":"owner@example.com","password":"password-1","IP":"198.51.100.77","UserAgent":"spoofed"}`},
		"telephone": {"/v1/auth/login/telephone", func(r *server.Hertz, s identity.Service) {
			r.POST("/v1/auth/login/telephone", TelephoneLoginHandler(s))
		}, `{"telephone":"13800138000","telephone_area_code":"86","password":"password-1"}`},
	} {
		t.Run(name, func(t *testing.T) {
			svc := &capturedService{}
			code, clientIP := serve(t, tc.path, func(r *server.Hertz) { tc.register(r, svc) }, tc.body)
			if code != xerr.SUCCESS || svc.calls != 1 {
				t.Fatalf("code = %d, calls = %d", code, svc.calls)
			}
			if svc.metadata.ClientIP != clientIP || svc.metadata.UserAgent != "Client-UA/1.0" {
				t.Fatalf("metadata = %+v, want %s and the raw User-Agent", svc.metadata, clientIP)
			}
		})
	}
}

// Every handler validates the request before the service, which applies the
// Turnstile check, runs.
func TestLoginHandlersValidateBeforeCallingTheService(t *testing.T) {
	logtest.Discard(t)
	svc := &capturedService{}
	code, _ := serve(t, "/v1/auth/login", func(r *server.Hertz) {
		r.POST("/v1/auth/login", UserLoginHandler(svc))
	}, `{"email":"not-an-email","password":"password-1"}`)
	if code != xerr.InvalidParams || svc.calls != 0 {
		t.Fatalf("code = %d, calls = %d; want a parameter error before the service", code, svc.calls)
	}
}
