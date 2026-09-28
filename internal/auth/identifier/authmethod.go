package identifier

import "strings"

const (
	Email  = "email"  //邮箱
	Mobile = "mobile" //手机
	Device = "device" //设备

)

func CanonicalEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func CanonicalIdentifier(authType, identifier string) string {
	if authType == Email {
		return CanonicalEmail(identifier)
	}
	return identifier
}

// EmailMailboxKey maps an email address to the mailbox that receives it: the
// subaddress after "+" is dropped, and Gmail ignores dots in the local part
// and treats googlemail.com as gmail.com. Addresses sharing a key reach one
// inbox, so one person could register them all. The key only detects such
// aliases; it is never stored or used to sign in.
func EmailMailboxKey(email string) string {
	email = CanonicalEmail(email)
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return email
	}
	local, domain := email[:at], email[at+1:]
	if plus := strings.Index(local, "+"); plus > 0 {
		local = local[:plus]
	}
	if domain == "googlemail.com" {
		domain = "gmail.com"
	}
	if domain == "gmail.com" {
		local = strings.ReplaceAll(local, ".", "")
	}
	return local + "@" + domain
}
