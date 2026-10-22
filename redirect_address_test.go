package sieve

import (
	"strings"
	"testing"
)

func redirectAddressLiteral(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

// checklist: SIEVE-CORE-39 SIEVE-ACT-03
func TestRFC5228RedirectAddressSyntax(t *testing.T) {
	invalid := []string{
		"", "user", "@example.org", "user@", "user@@example.org",
		"user name@example.org", "user@example.org trailing",
		"first@example.org, second@example.org", "<>",
		"Friends: user@example.org;", "Friends:;",
		`"Friends: work": user@example.org;`,
		`Friends (nested (comment: here)): user@example.org;`,
		"<@old.example,@relay.example:user@example.org>",
		"Name <@relay.example:user@example.org>",
		`"unclosed@example.org`, "user@[127.0.0.1", "user@example.org;",
		".user@example.org", "user..name@example.org", "user.@example.org",
	}
	for _, address := range invalid {
		t.Run(address, func(t *testing.T) {
			for _, prefix := range []string{"", `require "copy"; `} {
				copyTag := ""
				if prefix != "" {
					copyTag = ":copy "
				}
				for _, conditional := range []bool{false, true} {
					src := "redirect " + copyTag + redirectAddressLiteral(address) + ";"
					if conditional {
						src = "if false { " + src + " }"
					}
					src = prefix + src
					if _, err := Parse(src); err == nil {
						t.Errorf("Parse accepted invalid redirect: %s", src)
					}
					if err := Validate(src); err == nil {
						t.Errorf("Validate accepted invalid redirect: %s", src)
					}
				}
			}
		})
	}
}

// checklist: SIEVE-CORE-39 SIEVE-ACT-03
func TestRFC5228RedirectAddressPreserved(t *testing.T) {
	valid := []string{
		"user@example.org", "postmaster@example", "user+tag@example.org",
		"δοκιμή@παράδειγμα.δοκιμή", "User <user@bücher.example>",
		"Ops Team <ops@example.org>", `"Ops: team; friends" <ops@example.org>`,
		`"quoted: local; part"@example.org`, `"a\"b"@example.org`,
		`"a\\b"@example.org`, "user@[127.0.0.1]", "user@[IPv6:2001:db8::1]",
		"user@example.org (a: nested (comment; here))",
		`user@example.org (escaped \) comment: still here)`,
		`"Ops \"team: one\"" <ops@example.org>`,
	}
	for _, address := range valid {
		t.Run(address, func(t *testing.T) {
			for _, copy := range []bool{false, true} {
				prefix, tag := "", ""
				if copy {
					prefix, tag = `require "copy"; `, ":copy "
				}
				src := prefix + "redirect " + tag + redirectAddressLiteral(address) + ";"
				script, err := Parse(src)
				if err != nil {
					t.Fatalf("Parse rejected valid redirect: %v", err)
				}
				if err := Validate(src); err != nil {
					t.Fatalf("Validate rejected valid redirect: %v", err)
				}
				exec := newRecExec()
				out := script.Evaluate(sieveEmail(), exec)
				if out.Error != nil || out.ImplicitKeep != copy || len(exec.redirects) != 1 {
					t.Fatalf("outcome %+v, callbacks %+v", out, exec.redirects)
				}
				if got := exec.redirects[0]; got.addr != address || got.copy != copy {
					t.Errorf("callback = %+v, want address %q, copy %v", got, address, copy)
				}
			}
		})
	}
}

// checklist: SIEVE-CORE-39 SIEVE-ACT-03
func TestRFC5228RedirectDecodedAddress(t *testing.T) {
	for _, encoded := range []string{
		"Friends${hex:3a} user@example.org${hex:3b}",
		"first@example.org${hex:2c} second@example.org",
		"user${hex:40}${hex:40}example.org", "${unicode:40}example.org",
	} {
		src := `require "encoded-character"; redirect "` + encoded + `";`
		if _, err := Parse(src); err == nil {
			t.Errorf("Parse accepted decoded invalid address: %s", src)
		}
		if err := Validate(src); err == nil {
			t.Errorf("Validate accepted decoded invalid address: %s", src)
		}
	}
	script, err := Parse(`require "encoded-character"; redirect "user${hex:40}example.org";`)
	if err != nil {
		t.Fatal(err)
	}
	exec := newRecExec()
	out := script.Evaluate(sieveEmail(), exec)
	if out.Error != nil || len(exec.redirects) != 1 || exec.redirects[0].addr != "user@example.org" {
		t.Fatalf("outcome %+v, callbacks %+v", out, exec.redirects)
	}
}
