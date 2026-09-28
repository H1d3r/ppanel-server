package authmethodadmin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateAuthMethodConfig stores an administrator's configuration of an
// authentication method. A configuration that is not an object or does not
// decode is refused, not replaced by the defaults; a missing one resets the
// method to them.
func (s *Service) UpdateAuthMethodConfig(ctx context.Context, req *dto.UpdateAuthMethodConfigRequest) (*dto.AuthMethodConfig, error) {
	method, err := s.deps.Auths.FindOneByMethod(ctx, req.Method)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find auth method %q", req.Method)
	}

	mapping.DeepCopy(method, req)
	if req.Config != nil {
		if _, ok := req.Config.(map[string]any); !ok {
			return nil, fmt.Errorf("the %s config must be an object: %w", req.Method, xerr.NewErrCode(xerr.InvalidParams))
		}
		config, err := decodeMethodConfig(req.Method, req.Config)
		if err != nil {
			return nil, err
		}
		bytes, err := json.Marshal(config)
		if err != nil {
			return nil, xerr.Wrapf(err, xerr.ERROR, "marshal %s config", req.Method)
		}
		method.Config = string(bytes)
	} else {
		method.Config = initializePlatformConfig(req.Method)
	}
	if err := s.deps.Auths.Update(ctx, method); err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseUpdateError, "update auth method %q", req.Method)
	}

	resp := new(dto.AuthMethodConfig)
	mapping.DeepCopy(resp, method)
	if method.Config != "" {
		if err := json.Unmarshal([]byte(method.Config), &resp.Config); err != nil {
			return nil, xerr.Wrapf(err, xerr.ERROR, "decode stored %s config", req.Method)
		}
	}
	// The email, mobile and device settings are also held by the runtime
	// configuration, which reloads them.
	switch method.Method {
	case "email", "mobile", "device":
		s.deps.Reinitialize(method.Method)
	}
	return resp, nil
}

// decodeMethodConfig checks the configuration of the methods whose settings
// the runtime reads; the provider methods keep what the administrator sent.
func decodeMethodConfig(method string, config any) (any, error) {
	raw, err := json.Marshal(config)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.InvalidParams, "encode %s config", method)
	}
	switch method {
	case "email":
		emailConfig := new(auth.EmailAuthConfig)
		if err := emailConfig.Unmarshal(string(raw)); err != nil {
			return nil, xerr.Wrapf(err, xerr.InvalidParams, "invalid email config")
		}
		return emailConfig, nil
	case "mobile":
		mobileConfig := new(auth.MobileAuthConfig)
		if err := mobileConfig.Unmarshal(string(raw)); err != nil {
			return nil, xerr.Wrapf(err, xerr.InvalidParams, "invalid mobile config")
		}
		return mobileConfig, nil
	case "device":
		deviceConfig := new(auth.DeviceConfig)
		if err := deviceConfig.Unmarshal(string(raw)); err != nil {
			return nil, xerr.Wrapf(err, xerr.InvalidParams, "invalid device config")
		}
		if deviceConfig.OnlyRealDevice && !deviceConfig.EnableSecurity {
			return nil, fmt.Errorf("only_real_device requires enable_security: %w", xerr.NewErrCode(xerr.InvalidParams))
		}
		if deviceConfig.EnableSecurity && deviceConfig.SecuritySecret == "" {
			return nil, fmt.Errorf("device security secret is required: %w", xerr.NewErrCode(xerr.InvalidParams))
		}
		return deviceConfig, nil
	default:
		return config, nil
	}
}

// initializePlatformConfig is the default configuration of a method, as
// stored JSON; unknown methods have none.
func initializePlatformConfig(platform string) string {
	switch platform {
	case "email":
		return new(auth.EmailAuthConfig).Marshal()
	case "mobile":
		return new(auth.MobileAuthConfig).Marshal()
	case "apple":
		return new(auth.AppleAuthConfig).Marshal()
	case "google":
		return new(auth.GoogleAuthConfig).Marshal()
	case "github":
		return new(auth.GithubAuthConfig).Marshal()
	case "facebook":
		return new(auth.FacebookAuthConfig).Marshal()
	case "telegram":
		return new(auth.TelegramAuthConfig).Marshal()
	case "device":
		return new(auth.DeviceConfig).Marshal()
	}
	return ""
}
