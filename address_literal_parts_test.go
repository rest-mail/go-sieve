package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// checklist: SIEVE-CORE-50
func TestRFC5228AddressLiteralParts(t *testing.T) {
	// RFC 2822 dtext permits printable ASCII other than [ and ], with
	// backslash represented as a quoted-pair. Include @ at every position.
	domains := []string{"example.org", "[192.0.2.1]", "[IPv6:2001:db8::1]", "[@]", "[a@b]", "[a@@b@]", `[a@\[b]`, `[a@\]b]`, `[a@\\b]`}
	for c := byte(33); c <= 126; c++ {
		if c == '[' || c == ']' || c == '\\' {
			continue
		}
		domains = append(domains, "[@"+string(c)+"@]")
	}
	for _, local := range []string{"alice", `"a@b"`, `"a@[b]"`} {
		for _, domain := range domains {
			addr := local + "@" + domain
			for _, field := range []string{"From", "To", "Cc", "Bcc"} {
				msg := &Message{}
				addresses := []Address{{Address: addr}}
				switch field {
				case "From":
					msg.Headers.From = addresses
				case "To":
					msg.Headers.To = addresses
				case "Cc":
					msg.Headers.Cc = addresses
				case "Bcc":
					msg.Headers.Bcc = addresses
				}
				checkLiteralParts(t, `address`, field, msg, addr, local, domain)
			}
			for _, field := range []string{"from", "to"} {
				for _, route := range []string{"", "@hop.example:", "@[IPv6:2001:db8::1],@hop.example:"} {
					value := route + addr
					if route != "" {
						value = "<" + value + ">"
					}
					msg := &Message{Envelope: Envelope{From: value, To: []string{value}}}
					checkLiteralParts(t, `envelope`, field, msg, addr, local, domain)
				}
			}
		}
	}
}

func checkLiteralParts(t *testing.T, test, field string, msg *Message, addr, local, domain string) {
	t.Helper()
	quote := func(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }
	for part, want := range map[string]string{":all": addr, ":localpart": local, ":domain": domain} {
		expr := fmt.Sprintf(`%s %s :comparator "i;octet" %s %s`, test, part, quote(field), quote(want))
		if !matchedFolder(t, expr, msg) {
			t.Errorf("%s for %q did not match %q", test+" "+field+" "+part, addr, want)
		}
		// A split inside the literal must not become a false-positive match.
		if i := strings.LastIndexByte(addr, '@'); i >= 0 && addr[i+1:] != domain && part != ":all" {
			wrong := addr[:i]
			if part == ":domain" {
				wrong = addr[i+1:]
			}
			expr = fmt.Sprintf(`%s %s :comparator "i;octet" %s %s`, test, part, quote(field), quote(wrong))
			if matchedFolder(t, expr, msg) {
				t.Errorf("%s for %q falsely matched %q", test+" "+field+" "+part, addr, wrong)
			}
		}
	}
}
