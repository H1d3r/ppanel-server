package verifycode

import "github.com/redis/go-redis/v9"

// SmsCodeConfig is the configuration snapshot consumed by SMS code delivery.
type SmsCodeConfig struct {
	VerifyCodeInterval int64
	VerifyCodeLimit    int64
	VerifyCodeExpire   int64
	// WhitelistEnabled restricts delivery to the Whitelist area codes.
	WhitelistEnabled bool
	Whitelist        []string
}

// SendSmsCodeDependencies explicitly declares the collaborators of SMS code
// delivery instead of passing Application to business logic.
type SendSmsCodeDependencies struct {
	Store  VerificationIdentityStore
	Redis  *redis.Client
	Queue  VerificationTaskQueue
	Config SmsCodeConfig
	Policy VerificationCodePolicy
}
