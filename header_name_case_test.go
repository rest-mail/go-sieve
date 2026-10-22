package sieve

import (
	"fmt"
	"testing"
)

// Field names are ASCII protocol identifiers, independently of the comparator
// chosen for their values (RFC 5228 §§2.4.2.2, 5.1, 5.5, 5.7;
// RFC 5322 §3.6.8). Unicode look-alikes in the host's raw map are not aliases.
// checklist: SIEVE-CORE-46 SIEVE-TST-01 SIEVE-TST-06 SIEVE-TST-08
func TestRFC5228RawHeaderNameASCIICase(t *testing.T) {
	for _, tc := range []struct {
		raw, name string
		address   bool
		want      bool
	}{
		{"sUbJeCt", "SUBJECT", false, true},
		{"x-kEy", "X-KEY", false, true},
		{"sEnDeR", "SENDER", true, true},
		{"ReSeNt-SeNdEr", "resent-sender", true, true},
		{"RePlY-tO", "REPLY-TO", true, true},
		{"ſubject", "Subject", false, false},
		{"Subject", "ſubject", false, false},
		{"X-Key", "X-Key", false, false},
		{"X-Key", "X-Key", false, false},
		{"ſender", "Sender", true, false},
		{"Sender", "ſender", true, false},
		{"Reſent-From", "Resent-From", true, false},
		{"Resent-ſender", "Resent-Sender", true, false},
		{"Reply-To", "Reply-TO", true, true},
		{"Reply-To", "Reply-Tο", true, false}, // Greek omicron.
		{"Reply-To", "Reply-To ", true, false},
		{"Reply-To ", "Reply-To", true, false},
		{"Reply-To:", "Reply-To", true, false},
		{"Reply-To", "Reply-To:", true, false},
		{"Reply-To", "Reply-Tİ", true, false},
		{"Lİst-Id", "List-Id", false, false},
	} {
		t.Run(tc.raw+"/"+tc.name, func(t *testing.T) {
			msg := &Message{Headers: Headers{Raw: map[string][]string{tc.raw: {"hit@example.org"}}}}
			exprs := []string{fmt.Sprintf(`exists %q`, tc.name)}
			for _, comparator := range []string{"", `:comparator "i;octet"`, `:comparator "i;ascii-casemap"`} {
				exprs = append(exprs, fmt.Sprintf(`header %s :is %q "hit@example.org"`, comparator, tc.name))
				if tc.address {
					for _, part := range []struct{ tag, key string }{
						{":all", "hit@example.org"}, {":localpart", "hit"}, {":domain", "example.org"},
					} {
						exprs = append(exprs, fmt.Sprintf(`address %s %s :is %q %q`, comparator, part.tag, tc.name, part.key))
					}
				}
			}
			for _, expr := range exprs {
				if got := matchedFolder(t, expr, msg); got != tc.want {
					t.Errorf("%s with raw field %q: got %v, want %v", expr, tc.raw, got, tc.want)
				}
			}
		})
	}

	// A real field must neither hide nor inherit the value of a look-alike.
	msg := &Message{Headers: Headers{Raw: map[string][]string{
		"Subject": {"real"}, "ſubject": {"false"},
		"Sender": {"real@example.org"}, "ſender": {"false@example.org"},
	}}}
	for expr, want := range map[string]bool{
		`header :is "subject" "real"`:                       true,
		`header :is "subject" "false"`:                      false,
		`address :localpart :is "sender" "real"`:            true,
		`address :localpart :is "sender" "false"`:           false,
		`exists ["subject", "sender"]`:                      true,
		`exists ["subject", "X-Key"]`:                       false,
		`header :is ["X-Key", "subject"] ["false", "real"]`: true,
	} {
		if got := matchedFolder(t, expr, msg); got != want {
			t.Errorf("%s with coexisting fields: got %v, want %v", expr, got, want)
		}
	}
}

// checklist: SIEVE-CORE-46
func TestRFC5228RawHeaderNameOctets(t *testing.T) {
	for n := 0; n < 256; n++ {
		want := n == 's' || n == 'S'
		msg := &Message{Headers: Headers{Raw: map[string][]string{
			string([]byte{byte(n)}) + "ubject": {"hit"},
			string([]byte{byte(n)}) + "ender":  {"hit@example.org"},
		}}}
		for _, expr := range []string{
			`header :is "sUbJeCt" "hit"`,
			`exists "sUbJeCt"`,
			`address :is "sEnDeR" "hit@example.org"`,
		} {
			if got := matchedFolder(t, expr, msg); got != want {
				t.Errorf("raw field initial octet %02x, %s: got %v, want %v", n, expr, got, want)
			}
		}
	}
}
