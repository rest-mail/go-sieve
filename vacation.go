package sieve

import (
	"net/mail"
	"strings"
)

// vacationAllowed implements RFC 5230 §§4.5–4.6. The envelope supplies the
// recipient identity; :addresses adds aliases. The host must supply the final
// envelope recipient and preserve the original list/automation headers.
func (st *evalState) vacationAllowed(cmd *vacationCmd, replyTo string) bool {
	local, _, _ := strings.Cut(strings.ToLower(replyTo), "@")
	switch local {
	case "mailer-daemon", "listserv", "majordomo":
		return false
	}
	if strings.HasSuffix(local, "-request") || strings.HasPrefix(local, "owner-") {
		return false
	}
	for _, name := range []string{"List-Id", "List-Help", "List-Subscribe", "List-Unsubscribe", "List-Post", "List-Owner", "List-Archive"} {
		if len(headerValues(st.msg, name)) != 0 {
			return false
		}
	}
	for _, value := range headerValues(st.msg, "Auto-Submitted") {
		token, _, _ := strings.Cut(value, ";")
		if !strings.EqualFold(strings.TrimSpace(token), "no") {
			return false
		}
	}
	// RFC 5230 §4.6 permits additional heuristics; honor common bulk markers.
	for _, value := range headerValues(st.msg, "Precedence") {
		switch strings.ToLower(value) {
		case "bulk", "list", "junk":
			return false
		}
	}
	for _, name := range []string{"To", "Cc", "Bcc", "Resent-To", "Resent-Cc", "Resent-Bcc"} {
		for _, value := range headerValues(st.msg, name) {
			addresses, err := mail.ParseAddressList(value)
			if err != nil {
				continue
			}
			for _, address := range addresses {
				for _, identity := range st.msg.Envelope.To {
					if strings.EqualFold(address.Address, identity) {
						return true
					}
				}
				for _, identity := range cmd.addresses {
					if strings.EqualFold(address.Address, identity) {
						return true
					}
				}
			}
		}
	}
	return false
}
