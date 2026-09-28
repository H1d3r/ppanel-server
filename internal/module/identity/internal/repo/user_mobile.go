package repo

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"gorm.io/gorm"
)

// NormalizeMobileIdentifiers converts phone numbers stored in another form
// than E.164 to E.164, the form every sign-in, reset and verification code
// looks them up in. The admin panel used to store "<area>-<number>", which
// left those accounts unable to sign in or reset by phone. Formatting needs
// libphonenumber, so this runs at startup rather than as a SQL migration; a
// run over converted data changes nothing.
//
// A number whose E.164 form another binding already holds is left as it is
// and logged: it cannot take the number from the other binding, and the
// owner of that binding signs in with it already.
func (m *UserRepo) NormalizeMobileIdentifiers(ctx context.Context) (repository.MobileNormalization, error) {
	var result repository.MobileNormalization
	var candidates []*user.AuthMethods
	// E.164 is "+" followed by digits, so a stored number without the "+" or
	// with separators needs converting. The (auth_type, auth_identifier)
	// index serves the filter.
	err := m.QueryNoCacheCtx(ctx, &candidates, func(conn *gorm.DB, v any) error {
		return conn.Model(&user.AuthMethods{}).
			Where("auth_type = ?", identifier.Mobile).
			Where("(auth_identifier NOT LIKE ? OR auth_identifier LIKE ? OR auth_identifier LIKE ?)", "+%", "%-%", "% %").
			Order("id").
			Find(v).Error
	})
	if err != nil {
		return result, err
	}
	log := logger.WithContext(ctx)
	for _, method := range candidates {
		canonical, err := identifier.CanonicalMobile(method.AuthIdentifier)
		if err != nil {
			result.Unparsable++
			log.Errorw("[NormalizeMobileIdentifiers] stored phone number does not parse; left unchanged",
				logger.Field("auth_method_id", method.Id), logger.Field("user_id", method.UserId))
			continue
		}
		if canonical == method.AuthIdentifier {
			continue
		}
		var holders []user.AuthMethods
		err = m.QueryNoCacheCtx(ctx, &holders, func(conn *gorm.DB, v any) error {
			return queryAuthMethodsByExactIdentifier(conn, identifier.Mobile, canonical).Limit(1).Find(v).Error
		})
		if err != nil {
			return result, err
		}
		if len(holders) > 0 {
			result.Conflicts++
			log.Errorw("[NormalizeMobileIdentifiers] E.164 form already belongs to another binding; left unchanged",
				logger.Field("auth_method_id", method.Id), logger.Field("user_id", method.UserId),
				logger.Field("holder_auth_method_id", holders[0].Id), logger.Field("holder_user_id", holders[0].UserId),
				logger.Field("number", identifier.MaskPhoneNumber(canonical)))
			continue
		}
		converted := false
		err = m.ExecCtx(ctx, func(conn *gorm.DB) error {
			// The stored value is part of the condition, so a concurrent
			// startup converting the same row changes nothing twice.
			update := conn.Model(&user.AuthMethods{}).
				Where("id = ? AND auth_identifier = ?", method.Id, method.AuthIdentifier).
				Update("auth_identifier", canonical)
			converted = update.RowsAffected == 1
			return update.Error
		}, method.GetCacheKeys()...)
		if err != nil {
			// A concurrent binding of the same number wins the unique index.
			result.Conflicts++
			log.Errorw("[NormalizeMobileIdentifiers] convert phone number failed; left unchanged",
				logger.Field("auth_method_id", method.Id), logger.Field("user_id", method.UserId), logger.Field("error", err.Error()))
			continue
		}
		if converted {
			result.Converted++
		}
	}
	return result, nil
}
