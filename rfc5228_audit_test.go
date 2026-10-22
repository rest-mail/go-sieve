package sieve

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// checklist: SIEVE-CORE-01
func TestRFC5228MinimumIntegerRange(t *testing.T) {
	for _, n := range []int64{0, 1, 2147483646, 2147483647} {
		for _, over := range []bool{false, true} {
			tag := ":under"
			if over {
				tag = ":over"
			}
			for _, delta := range []int64{-1, 0, 1} {
				size := n + delta
				if size <= 0 {
					continue
				} // RawSize=0 requests reconstruction.
				want := size < n
				if over {
					want = size > n
				}
				expr := fmt.Sprintf("size %s %d", tag, n)
				if got := matchedFolder(t, expr, &Message{RawSize: size}); got != want {
					t.Errorf("%s at %d: got %v, want %v", expr, size, got, want)
				}
			}
			if err := Validate(fmt.Sprintf("if size %s %d { keep; }", tag, n)); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// checklist: SIEVE-CORE-02
func TestRFC5228OpaqueStringOctets(t *testing.T) {
	// Every legal single octet survives quoted and multiline string parsing.
	for n := 1; n <= 255; n++ {
		if n == '\r' || n == '\n' {
			continue
		}
		value := string([]byte{byte(n)})
		quoted := value
		if n == '"' || n == '\\' {
			quoted = "\\" + value
		}
		for _, literal := range []string{"\"" + quoted + "\"", "text:\r\n" + strings.ReplaceAll(value, ".", "..") + "\r\n.\r\n"} {
			want := value
			if strings.HasPrefix(literal, "text:") {
				want += "\r\n"
			}
			s, err := Parse(`require "reject"; reject ` + literal + `;`)
			if err != nil {
				t.Fatalf("octet %d: %v", n, err)
			}
			got := s.Evaluate(&Message{}, newRecExec())
			if got.Disposition != Reject || got.RejectReason != want || got.Error != nil {
				t.Fatalf("octet %d: %+v, want %q", n, got, want)
			}
		}
	}
}

// checklist: SIEVE-CORE-03
func TestRFC5228InvalidHeaderNamesNeverMatch(t *testing.T) {
	for _, name := range []string{"", "Subject:", " Subject", "Subject ", "Bad Name", "Bad\tName", "Bad\r\nName", "Bäd"} {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			msg := &Message{Headers: Headers{Subject: "hit", Raw: map[string][]string{name: {"hit"}}}}
			// Use Sieve encoded-character for control octets, not Go-only escapes.
			literal := `"${hex:`
			for _, b := range []byte(name) {
				literal += fmt.Sprintf(" %02x", b)
			}
			if name == "" {
				literal = `""`
			} else {
				literal += `}"`
			}
			for _, expr := range []string{`header :contains ` + literal + ` ""`, `exists ` + literal, `address ` + literal + ` "hit@example.org"`} {
				s, err := Parse(`require "encoded-character"; if ` + expr + ` { discard; }`)
				if err != nil {
					t.Fatal(err)
				}
				out := s.Evaluate(msg, newRecExec())
				if out.Error != nil || out.Disposition != Continue || !out.ImplicitKeep {
					t.Errorf("%s: %+v", expr, out)
				}
			}
		})
	}
	if !matchedFolder(t, `header :is "sUbJeCt" "hit"`, &Message{Headers: Headers{Subject: "hit"}}) {
		t.Fatal("valid control did not match")
	}
}

// checklist: SIEVE-CORE-07
func TestRFC5228EncodedNULDoesNotTruncate(t *testing.T) {
	msg := &Message{Headers: Headers{Subject: "a\x00tail"}}
	for expr, want := range map[string]bool{
		`header :is "Subject" "a${hex:00}tail"`:  true,
		`header :contains "Subject" "tail"`:      true,
		`header :is "Subject" "a"`:               false,
		`header :is "Subject" "a${hex:00}wrong"`: false,
	} {
		s, err := Parse(`require "encoded-character"; if ` + expr + ` { discard; }`)
		if err != nil {
			t.Fatal(err)
		}
		out := s.Evaluate(msg, newRecExec())
		if (out.Disposition == Discard) != want || out.Error != nil {
			t.Errorf("%s: %+v", expr, out)
		}
	}
}

// checklist: SIEVE-CORE-09 SIEVE-CORE-10
func TestRFC5228RepeatedDeliveryIsNotError(t *testing.T) {
	for _, src := range []string{`keep;`, `keep; keep;`, `require "fileinto"; fileinto "box";`, `require "fileinto"; fileinto "box"; fileinto "box";`} {
		s, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		exec := newRecExec()
		out := s.Evaluate(&Message{}, exec)
		if out.Error != nil || out.Disposition != Continue || out.ImplicitKeep || len(exec.applied) != 1 {
			t.Errorf("%s: %+v, actions %v", src, out, exec.applied)
		}
	}
}

// checklist: SIEVE-CORE-11
func TestRFC5228ChecksExecutedCode(t *testing.T) {
	for _, src := range []string{`if true { keep "extra"; }`, `if true { redirect; }`, `if true { unknown; }`, `require "unsupported"; discard;`, `if true { fileinto "box"; }`} {
		s, err := Parse(src)
		if err == nil || s != nil {
			t.Errorf("invalid executable code accepted: %q, script %v, error %v", src, s, err)
		}
	}
}

// checklist: SIEVE-CORE-13 SIEVE-CORE-14
func TestRFC5228FifteenNestingLevels(t *testing.T) {
	for _, src := range []string{
		strings.Repeat("if true { ", 15) + "keep;" + strings.Repeat(" }", 15),
		"if " + strings.Repeat("allof (", 15) + "true" + strings.Repeat(")", 15) + " { keep; }",
		"if " + strings.Repeat("anyof (", 15) + "true" + strings.Repeat(")", 15) + " { keep; }",
	} {
		s, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		exec := newRecExec()
		out := s.Evaluate(&Message{}, exec)
		if out.Error != nil || out.Disposition != Continue || len(exec.applied) != 1 || exec.applied[0] != "keep" {
			t.Fatalf("fifteen levels: %+v, %v", out, exec.applied)
		}
	}
}

// checklist: SIEVE-CORE-21
func TestRFC5228DiscardIsSilent(t *testing.T) {
	s, err := Parse(`discard;`)
	if err != nil {
		t.Fatal(err)
	}
	exec := newRecExec()
	out := s.Evaluate(&Message{}, exec)
	if out.Disposition != Discard || out.Error != nil || out.RejectReason != "" || out.ImplicitKeep || len(exec.applied) != 0 {
		t.Fatalf("discard generated another action: %+v, %v", out, exec.applied)
	}
}

// checklist: SIEVE-CORE-22 SIEVE-CORE-41
func TestRFC5228AddressHeaderRestriction(t *testing.T) {
	for _, name := range []string{"From", "To", "Cc", "Bcc", "Sender", "Resent-From", "Resent-To", "Reply-To", "Resent-Cc", "Resent-Bcc", "Resent-Sender", "Subject", "X-Text"} {
		t.Run(name, func(t *testing.T) {
			msg := &Message{Headers: Headers{Raw: map[string][]string{name: {"Alice <alice@example.org>"}}}}
			want := name != "Subject" && name != "X-Text"
			if got := matchedFolder(t, `address :is "`+name+`" "alice@example.org"`, msg); got != want {
				t.Errorf("address match %v, want %v", got, want)
			}
			if !matchedFolder(t, `header :contains "`+name+`" "alice@example.org"`, msg) {
				t.Fatal("header control failed")
			}
		})
	}
}

// checklist: SIEVE-CORE-23
func TestRFC5228EnvelopeDropsSourceRoutes(t *testing.T) {
	for _, route := range []string{"@hop.example:", "@one.example,@two.example:", "@[IPv6:2001:db8::1]:"} {
		for _, wrap := range []bool{false, true} {
			addr := route + "alice@example.org"
			if wrap {
				addr = "<" + addr + ">"
			}
			msg := &Message{Envelope: Envelope{From: addr, To: []string{addr}}}
			for _, field := range []string{"from", "to"} {
				for part, key := range map[string]string{":all": "alice@example.org", ":localpart": "alice", ":domain": "example.org"} {
					if !matchedFolder(t, `envelope `+part+` :is "`+field+`" "`+key+`"`, msg) {
						t.Errorf("%s %s failed for %s", field, part, addr)
					}
				}
				if matchedFolder(t, `envelope :all :contains "`+field+`" "hop.example"`, msg) {
					t.Error("source route leaked into comparison")
				}
			}
		}
	}
	// A colon inside a quoted local-part or address literal is not a route.
	for _, addr := range []string{`"a:b"@example.org`, `alice@[IPv6:2001:db8::1]`} {
		if !matchedFolder(t, `envelope :all :is "from" `+strconv.Quote(addr), &Message{Envelope: Envelope{From: addr}}) {
			t.Errorf("non-route changed: %s", addr)
		}
	}
}

// checklist: SIEVE-CORE-25
func TestRFC5228CapabilityStringsCaseSensitive(t *testing.T) {
	for _, cap := range []string{"FILEINTO", "Fileinto", "Envelope", "ENCODED-CHARACTER", "Comparator-i;octet"} {
		s, err := Parse(`require "` + cap + `"; discard;`)
		if err == nil || s != nil {
			t.Errorf("case variant %q accepted", cap)
		}
	}
	if err := Validate(`REQUIRE ["fileinto", "envelope", "encoded-character"]; keep;`); err != nil {
		t.Fatal(err)
	}
}

// checklist: SIEVE-CORE-38
func TestRFC5228MultilinePreservesSingleDotAndCRLF(t *testing.T) {
	for _, ending := range []string{"\r\n", "\n"} {
		src := `require "reject"; reject text:` + ending + ".single" + ending + "..double" + ending + "...triple" + ending + "." + ending + ";"
		s, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		out := s.Evaluate(&Message{}, newRecExec())
		want := ".single" + ending + ".double" + ending + "..triple" + ending
		if out.Disposition != Reject || out.RejectReason != want || out.Error != nil {
			t.Fatalf("ending %q: %+v, want %q", ending, out, want)
		}
	}
}

// checklist: SIEVE-CORE-15
func TestRFC5228MandatoryCoreSupport(t *testing.T) {
	for _, expr := range []string{
		`address "From" "alice@example.org"`, `allof (true, true)`,
		`anyof (false, true)`, `exists "From"`, `not false`,
		`header "Subject" "hit"`, `not not true`, `size :over 1`, `true`,
	} {
		msg := &Message{Headers: Headers{From: []Address{{Address: "alice@example.org"}}, Subject: "hit"}, RawSize: 20}
		if !matchedFolder(t, expr, msg) {
			t.Errorf("required test failed: %s", expr)
		}
	}
	for _, action := range []string{`keep;`, `discard;`, `redirect "alice@example.org";`} {
		s, err := Parse(action)
		if err != nil {
			t.Fatal(err)
		}
		exec := newRecExec()
		out := s.Evaluate(&Message{}, exec)
		if out.Error != nil || out.ImplicitKeep {
			t.Fatalf("%s: %+v", action, out)
		}
		switch action {
		case `keep;`:
			if out.Disposition != Continue || strings.Join(exec.applied, ",") != "keep" {
				t.Fatal(out, exec.applied)
			}
		case `discard;`:
			if out.Disposition != Discard || len(exec.applied) != 0 {
				t.Fatal(out, exec.applied)
			}
		default:
			if out.Disposition != Continue || strings.Join(exec.applied, ",") != "redirect:alice@example.org" {
				t.Fatal(out, exec.applied)
			}
		}
	}
}

type auditFailingExecutor struct{ *recExec }

func (e auditFailingExecutor) FileInto(folder string, create bool) {
	e.recExec.FileInto(folder, create)
	panic("disk full")
}

// checklist: SIEVE-CORE-11
func TestRFC5228RuntimeChecksStopProcessing(t *testing.T) {
	s, err := Parse(`require "fileinto"; keep; fileinto "full"; redirect "never@example.org";`)
	if err != nil {
		t.Fatal(err)
	}
	exec := auditFailingExecutor{newRecExec()}
	out := s.Evaluate(&Message{}, exec)
	if out.Error == nil || !strings.Contains(out.Error.Error(), "disk full") || !out.ImplicitKeep || out.Disposition != Continue {
		t.Fatal(out)
	}
	if got := strings.Join(exec.applied, ","); got != "keep,fileinto:full" {
		t.Fatalf("processing continued after failure or lost prior action record: %s", got)
	}
}
