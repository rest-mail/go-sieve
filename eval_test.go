package sieve

import (
	"strconv"
	"strings"
	"testing"
)

// ── Test harness ─────────────────────────────────────────────────────

// recExec is a reference Executor that records the actions a script selects
// into a metadata map, the way a mail host would apply them, so the evaluator's
// behaviour can be asserted directly.
type recExec struct {
	meta    map[string]string
	applied []string
}

func newRecExec() *recExec { return &recExec{meta: map[string]string{}} }

func (e *recExec) Keep() { e.applied = append(e.applied, "keep") }

func (e *recExec) FileInto(folder string, create bool) {
	e.meta["deliver_to_folder"] = folder
	if create {
		e.meta["deliver_to_folder_create"] = "true"
	}
	e.applied = append(e.applied, "fileinto:"+folder)
}

func (e *recExec) Redirect(addr string) {
	e.meta["redirect_to"] = addr
	e.applied = append(e.applied, "redirect:"+addr)
}

func (e *recExec) Flag(op string, flags []string) {
	applyFlags(e.meta, op, flags)
	e.applied = append(e.applied, op+":"+strings.Join(flags, " "))
}

func (e *recExec) Vacation(v Vacation) {
	e.meta["vacation_reply_to"] = v.ReplyTo
	e.meta["vacation_reply_subject"] = v.Subject
	e.meta["vacation_reply_body"] = v.Body
	if v.Days > 0 {
		e.meta["vacation_days"] = strconv.Itoa(v.Days)
	}
	e.applied = append(e.applied, "vacation:"+v.ReplyTo)
}

func (e *recExec) Notify(method, message string) {
	e.meta["notify_method"] = method
	e.meta["notify_message"] = message
	e.applied = append(e.applied, "notify:"+method)
}

// applyFlags updates the imap4flags flag set stored in the meta map, mirroring a
// host's imap4flags handling.
func applyFlags(meta map[string]string, op string, flags []string) {
	var current []string
	if existing := strings.TrimSpace(meta["imap_flags"]); existing != "" {
		current = strings.Fields(existing)
	}
	switch op {
	case "setflag":
		current = uniqueStrings(flags)
	case "addflag":
		current = uniqueStrings(append(current, flags...))
	case "removeflag":
		remove := make(map[string]struct{}, len(flags))
		for _, fl := range flags {
			remove[fl] = struct{}{}
		}
		kept := current[:0]
		for _, fl := range current {
			if _, drop := remove[fl]; !drop {
				kept = append(kept, fl)
			}
		}
		current = kept
	}
	meta["imap_flags"] = strings.Join(current, " ")
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// sieveResult bundles the recorded actions and terminal outcome of a run.
type sieveResult struct {
	meta        map[string]string
	disposition Disposition
	reject      string
}

func runSieve(t *testing.T, script string, msg *Message) *sieveResult {
	t.Helper()
	s, err := Parse(script)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ex := newRecExec()
	out := s.Evaluate(msg, ex)
	return &sieveResult{meta: ex.meta, disposition: out.Disposition, reject: out.RejectReason}
}

// folderOf returns the fileinto target recorded on a result, or "".
func folderOf(r *sieveResult) string { return r.meta["deliver_to_folder"] }

func sieveEmail() *Message {
	return &Message{
		Envelope: Envelope{
			From: "sender@example.com",
			To:   []string{"recipient@example.com"},
		},
		Headers: Headers{
			From:      []Address{{Address: "sender@example.com"}},
			To:        []Address{{Address: "recipient@example.com"}},
			Date:      "Mon, 17 Feb 2026 10:00:00 +0000",
			MessageID: "<abc123@example.com>",
			Subject:   "Test message",
		},
		Body: Body{
			ContentType: "text/plain",
			Content:     "Hello, this is a test message body.",
		},
	}
}

// ── if / elsif / else ────────────────────────────────────────────────

func TestSieve_IfElsifElse(t *testing.T) {
	script := `require "fileinto";
if header :contains "Subject" "invoice" {
  fileinto "Invoices";
} elsif header :contains "Subject" "receipt" {
  fileinto "Receipts";
} else {
  fileinto "Other";
}`

	cases := []struct {
		subject string
		want    string
	}{
		{"Your invoice is ready", "Invoices"},
		{"Payment receipt", "Receipts"},
		{"Random newsletter", "Other"},
	}
	for _, tc := range cases {
		email := sieveEmail()
		email.Headers.Subject = tc.subject
		if got := folderOf(runSieve(t, script, email)); got != tc.want {
			t.Errorf("subject %q: expected folder %q, got %q", tc.subject, tc.want, got)
		}
	}
}

func TestSieve_ElsifOnlyFirstMatchRuns(t *testing.T) {
	// Both branches would match; only the first should fire.
	script := `require "fileinto";
if header :contains "Subject" "test" {
  fileinto "First";
} elsif header :contains "Subject" "message" {
  fileinto "Second";
}`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "First" {
		t.Errorf("expected only first branch to run, got %q", got)
	}
}

// ── allof / anyof / not ──────────────────────────────────────────────

func TestSieve_Allof(t *testing.T) {
	script := `require "fileinto";
if allof (header :contains "Subject" "test", address :is "From" "sender@example.com") {
  fileinto "Both";
}`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "Both" {
		t.Errorf("expected allof match, got %q", got)
	}
}

func TestSieve_Allof_OneFalse(t *testing.T) {
	script := `require "fileinto";
if allof (header :contains "Subject" "test", address :is "From" "nobody@example.com") {
  fileinto "Both";
}`
	if got := folderOf(runSieve(t, script, sieveEmail())); got == "Both" {
		t.Error("allof should not match when one test is false")
	}
}

func TestSieve_Anyof(t *testing.T) {
	script := `require "fileinto";
if anyof (header :contains "Subject" "nope", header :contains "Subject" "test") {
  fileinto "Any";
}`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "Any" {
		t.Errorf("expected anyof match, got %q", got)
	}
}

func TestSieve_Anyof_NoneTrue(t *testing.T) {
	script := `require "fileinto";
if anyof (header :contains "Subject" "nope", header :contains "Subject" "nada") {
  fileinto "Any";
}`
	if got := folderOf(runSieve(t, script, sieveEmail())); got == "Any" {
		t.Error("anyof should not match when all tests are false")
	}
}

func TestSieve_Not(t *testing.T) {
	script := `require "fileinto";
if not header :contains "Subject" "spam" {
  fileinto "Ham";
}`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "Ham" {
		t.Errorf("expected not-match, got %q", got)
	}
}

func TestSieve_NotAnyofNested(t *testing.T) {
	script := `require "fileinto";
if not anyof (header :contains "Subject" "spam", header :contains "Subject" "junk") {
  fileinto "Clean";
}`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "Clean" {
		t.Errorf("expected nested not/anyof match, got %q", got)
	}
}

// ── true / false ─────────────────────────────────────────────────────

func TestSieve_TrueFalse(t *testing.T) {
	if got := folderOf(runSieve(t, `require "fileinto";
if true { fileinto "Yes"; }`, sieveEmail())); got != "Yes" {
		t.Errorf("true test should always match, got %q", got)
	}
	if got := folderOf(runSieve(t, `require "fileinto";
if false { fileinto "No"; }`, sieveEmail())); got == "No" {
		t.Error("false test should never match")
	}
}

// ── exists ───────────────────────────────────────────────────────────

func TestSieve_Exists(t *testing.T) {
	script := `require "fileinto";
if exists "Subject" { fileinto "HasSubject"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "HasSubject" {
		t.Errorf("expected exists match, got %q", got)
	}
}

func TestSieve_Exists_AllRequired(t *testing.T) {
	// exists is true only if every listed header is present.
	script := `require "fileinto";
if exists ["Subject", "X-Missing"] { fileinto "Both"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got == "Both" {
		t.Error("exists must be false when one header is absent")
	}
}

func TestSieve_Exists_RawHeader(t *testing.T) {
	script := `require "fileinto";
if exists "X-Custom" { fileinto "Custom"; }`
	email := sieveEmail()
	email.Headers.Raw = map[string][]string{"X-Custom": {"hello"}}
	if got := folderOf(runSieve(t, script, email)); got != "Custom" {
		t.Errorf("expected raw header exists match, got %q", got)
	}
}

// ── size ─────────────────────────────────────────────────────────────

func TestSieve_Size(t *testing.T) {
	email := sieveEmail() // body is a few dozen bytes
	if got := folderOf(runSieve(t, `require "fileinto";
if size :over 5 { fileinto "Big"; }`, email)); got != "Big" {
		t.Errorf("expected size :over 5 to match, got %q", got)
	}
	if got := folderOf(runSieve(t, `require "fileinto";
if size :under 5 { fileinto "Small"; }`, sieveEmail())); got == "Small" {
		t.Error("size :under 5 should not match a larger body")
	}
}

func TestSieve_Size_Quantifier(t *testing.T) {
	email := sieveEmail()
	email.Body.Content = strings.Repeat("x", 2048) // 2 KiB
	if got := folderOf(runSieve(t, `require "fileinto";
if size :over 1K { fileinto "Over1K"; }`, email)); got != "Over1K" {
		t.Errorf("expected size :over 1K to match 2KiB body, got %q", got)
	}
	small := sieveEmail()
	if got := folderOf(runSieve(t, `require "fileinto";
if size :over 1K { fileinto "Over1K"; }`, small)); got == "Over1K" {
		t.Error("small body should not exceed 1K")
	}
}

func TestSieve_Size_IncludesAttachments(t *testing.T) {
	email := sieveEmail()
	email.Attachments = []Attachment{{Size: 5000}}
	if got := folderOf(runSieve(t, `require "fileinto";
if size :over 4K { fileinto "Heavy"; }`, email)); got != "Heavy" {
		t.Errorf("expected attachment bytes to count toward size, got %q", got)
	}
}

// sizedEmail returns a message whose body alone (50 octets) is well under 100,
// but whose full wire size — the header block, the blank separator line, and the
// body — is well over it. Per RFC 5228 §5.9 the size test counts the entire
// message, so both :over and :under must reflect the header octets.
func sizedEmail() *Message {
	return &Message{
		Headers: Headers{
			Subject: strings.Repeat("A", 200), // header block >> body
			From:    []Address{{Address: "sender@example.com"}},
			To:      []Address{{Address: "recipient@example.com"}},
		},
		Body: Body{
			ContentType: "text/plain",
			Content:     strings.Repeat("x", 50), // 50-octet body
		},
	}
}

func TestSieve_Size_IncludesHeaders(t *testing.T) {
	// :over — false when only the body (50 octets) is counted, true once the
	// header octets are included and the whole message exceeds 100.
	if got := folderOf(runSieve(t, `require "fileinto";
if size :over 100 { fileinto "Big"; }`, sizedEmail())); got != "Big" {
		t.Errorf("size :over 100 must count header octets, got %q", got)
	}
	// :under — matches when only the body is counted (50 < 100), but must not
	// match once the header octets push the whole message past 100.
	if got := folderOf(runSieve(t, `require "fileinto";
if size :under 100 { fileinto "Small"; }`, sizedEmail())); got == "Small" {
		t.Error("size :under 100 must count header octets and not match")
	}
}

func TestSieve_Size_UsesRawSizeWhenSet(t *testing.T) {
	// A host-supplied exact wire size takes precedence over the reconstruction.
	big := sieveEmail()
	big.RawSize = 100000
	if got := folderOf(runSieve(t, `require "fileinto";
if size :over 4K { fileinto "Big"; }`, big)); got != "Big" {
		t.Errorf("size test should use host-supplied RawSize, got %q", got)
	}
	// A small RawSize wins even against a large body and attachments.
	small := sieveEmail()
	small.Body.Content = strings.Repeat("x", 10000)
	small.Attachments = []Attachment{{Size: 999999}}
	small.RawSize = 200
	if got := folderOf(runSieve(t, `require "fileinto";
if size :under 1K { fileinto "Small"; }`, small)); got != "Small" {
		t.Errorf("RawSize must override the reconstructed size, got %q", got)
	}
}

// ── :matches wildcards ───────────────────────────────────────────────

func TestSieve_MatchesStar(t *testing.T) {
	script := `require "fileinto";
if header :matches "Subject" "Test *" { fileinto "M"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "M" {
		t.Errorf("expected '*' wildcard match, got %q", got)
	}
}

func TestSieve_MatchesQuestionMark(t *testing.T) {
	script := `require "fileinto";
if header :matches "Subject" "Te?t message" { fileinto "Q"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "Q" {
		t.Errorf("expected '?' wildcard match, got %q", got)
	}
}

func TestSieve_MatchesAnchored(t *testing.T) {
	// :matches is anchored: a partial pattern must not match.
	script := `require "fileinto";
if header :matches "Subject" "message" { fileinto "NoMatch"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got == "NoMatch" {
		t.Error(":matches must be anchored (full-string) and not match a substring")
	}
}

func TestSieve_MatchesEscapedWildcard(t *testing.T) {
	// "\\*" in the source becomes "\*" after string parsing, a literal star.
	email := sieveEmail()
	email.Headers.Subject = "50% off*"
	script := `require "fileinto";
if header :matches "Subject" "50% off\\*" { fileinto "Literal"; }`
	if got := folderOf(runSieve(t, script, email)); got != "Literal" {
		t.Errorf("expected escaped literal-star match, got %q", got)
	}
}

// ── address parts ────────────────────────────────────────────────────

func TestSieve_AddressLocalpart(t *testing.T) {
	script := `require "fileinto";
if address :localpart :is "From" "sender" { fileinto "Local"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "Local" {
		t.Errorf("expected :localpart match, got %q", got)
	}
}

func TestSieve_AddressDomain(t *testing.T) {
	script := `require "fileinto";
if address :domain :is "From" "example.com" { fileinto "Domain"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "Domain" {
		t.Errorf("expected :domain match, got %q", got)
	}
}

func TestSieve_AddressAllDefault(t *testing.T) {
	script := `require "fileinto";
if address :is "From" "sender@example.com" { fileinto "All"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "All" {
		t.Errorf("expected default :all address match, got %q", got)
	}
}

func TestSieve_AddressCc(t *testing.T) {
	script := `require "fileinto";
if address :domain :is "Cc" "cc.example.com" { fileinto "CcDomain"; }`
	email := sieveEmail()
	email.Headers.Cc = []Address{{Address: "someone@cc.example.com"}}
	if got := folderOf(runSieve(t, script, email)); got != "CcDomain" {
		t.Errorf("expected Cc domain match, got %q", got)
	}
}

func TestSieve_EnvelopeDomainPart(t *testing.T) {
	script := `require ["envelope", "fileinto"];
if envelope :domain :is "from" "example.com" { fileinto "EnvDom"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "EnvDom" {
		t.Errorf("expected envelope :domain match, got %q", got)
	}
}

// ── address parts on non-structured (raw) headers ────────────────────
//
// Regression for #11: the address test on a header outside the structured
// set (From/To/Cc/Bcc) went through the raw map and did string surgery on the
// unparsed header value. It must instead parse the RFC 5322 address and derive
// :all/:localpart/:domain from the addr-spec, excluding display names/comments.

func TestSieve_AddressRawHeader_DisplayName(t *testing.T) {
	email := sieveEmail()
	// Quoted display name that itself looks like an address, plus the real
	// addr-spec c@d. Old code split the whole raw value on the last '@'.
	email.Headers.Raw = map[string][]string{"Resent-From": {`"a@b" <c@d>`}}

	// :domain must be "d"; string surgery yielded "d>" (trailing bracket).
	if got := folderOf(runSieve(t, `require "fileinto";
if address :domain :is "Resent-From" "d" { fileinto "Dom"; }`, email)); got != "Dom" {
		t.Errorf(`:domain should parse to "d", got folder %q`, got)
	}
	// :localpart must be "c"; string surgery yielded `"a@b" <c`.
	if got := folderOf(runSieve(t, `require "fileinto";
if address :localpart :is "Resent-From" "c" { fileinto "Loc"; }`, email)); got != "Loc" {
		t.Errorf(`:localpart should parse to "c", got folder %q`, got)
	}
	// The display-name text ("a@b") must NOT be matched: it is not an address.
	if got := folderOf(runSieve(t, `require "fileinto";
if address :all :contains "Resent-From" "a@b" { fileinto "Display"; }`, email)); got == "Display" {
		t.Error(":all must not match display-name text")
	}
}

func TestSieve_AddressRawHeader_AngleAddr(t *testing.T) {
	email := sieveEmail()
	// Quoted local-part containing '@', wrapped in angle brackets.
	email.Headers.Raw = map[string][]string{"Resent-Sender": {`<"weird@local"@example.com>`}}
	// Old code split the raw value on the last '@' and returned "example.com>".
	if got := folderOf(runSieve(t, `require "fileinto";
if address :domain :is "Resent-Sender" "example.com" { fileinto "Dom"; }`, email)); got != "Dom" {
		t.Errorf(`:domain should parse to "example.com", got folder %q`, got)
	}
}

func TestSieve_AddressRawHeader_Unparseable(t *testing.T) {
	email := sieveEmail()
	email.Headers.Raw = map[string][]string{"Resent-From": {"not an address"}}
	// A value that is not an addr-spec must not match :localpart (RFC 5228 §2.7.4);
	// string surgery returned the whole string as the local part.
	if got := folderOf(runSieve(t, `require "fileinto";
if address :localpart :is "Resent-From" "not an address" { fileinto "Bad"; }`, email)); got == "Bad" {
		t.Error("unparseable address must not match :localpart")
	}
}

// ── comparators ──────────────────────────────────────────────────────

func TestSieve_ComparatorOctetCaseSensitive(t *testing.T) {
	// Subject is "Test message"; octet comparison is case-sensitive.
	script := `require "fileinto";
if header :is :comparator "i;octet" "Subject" "test message" { fileinto "NoMatch"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got == "NoMatch" {
		t.Error("i;octet :is must be case-sensitive")
	}
}

func TestSieve_ComparatorCasemapDefault(t *testing.T) {
	script := `require "fileinto";
if header :is "Subject" "test message" { fileinto "CI"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "CI" {
		t.Errorf("expected default casemap to match case-insensitively, got %q", got)
	}
}

func TestSieve_ComparatorAsciiNumeric(t *testing.T) {
	email := sieveEmail()
	email.Headers.Raw = map[string][]string{"X-Priority": {"1"}}
	match := `require ["fileinto", "comparator-i;ascii-numeric"];
if header :is :comparator "i;ascii-numeric" "X-Priority" "1" { fileinto "Prio"; }`
	if got := folderOf(runSieve(t, match, email)); got != "Prio" {
		t.Errorf("expected ascii-numeric equality, got %q", got)
	}
	noMatch := `require ["fileinto", "comparator-i;ascii-numeric"];
if header :is :comparator "i;ascii-numeric" "X-Priority" "2" { fileinto "Prio"; }`
	if got := folderOf(runSieve(t, noMatch, email)); got == "Prio" {
		t.Error("ascii-numeric 1 should not equal 2")
	}
}

// ── header whitespace / unfolding ────────────────────────────────────

func TestSieve_HeaderTrimsWhitespace(t *testing.T) {
	// RFC 5228 §5.7: the header test ignores leading and trailing whitespace
	// in the header value. A value padded with spaces must still match :is.
	email := sieveEmail()
	email.Headers.Raw = map[string][]string{"X-Test": {"   foo   "}}
	script := `require "fileinto";
if header :is "X-Test" "foo" { fileinto "Trimmed"; }`
	if got := folderOf(runSieve(t, script, email)); got != "Trimmed" {
		t.Errorf("expected whitespace-padded header to match :is, got %q", got)
	}
}

func TestSieve_HeaderTrimsWhitespaceContains(t *testing.T) {
	// :contains against a leading-space value: with the value trimmed, an
	// anchored key that includes the leading padding must no longer match.
	email := sieveEmail()
	email.Headers.Raw = map[string][]string{"X-Test": {"   foo"}}
	script := `require "fileinto";
if header :contains "X-Test" "  foo" { fileinto "Padded"; }`
	if got := folderOf(runSieve(t, script, email)); got == "Padded" {
		t.Error("leading whitespace should be stripped before :contains")
	}
}

func TestSieve_HeaderUnfoldsContinuation(t *testing.T) {
	// RFC 5322 §2.2.3 unfolding: a folded header (CRLF + folding whitespace)
	// is a single logical value. header :is must compare against the unfolded
	// value with the fold collapsed to a single space, not the raw CRLF/WSP.
	email := sieveEmail()
	email.Headers.Raw = map[string][]string{"X-Test": {"foo\r\n bar"}}
	script := `require "fileinto";
if header :is "X-Test" "foo bar" { fileinto "Unfolded"; }`
	if got := folderOf(runSieve(t, script, email)); got != "Unfolded" {
		t.Errorf("expected folded header to unfold before matching, got %q", got)
	}
}

// ── string lists ─────────────────────────────────────────────────────

func TestSieve_KeyList(t *testing.T) {
	script := `require "fileinto";
if header :contains "Subject" ["foo", "test", "bar"] { fileinto "List"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "List" {
		t.Errorf("expected match against one of a key list, got %q", got)
	}
}

func TestSieve_HeaderList(t *testing.T) {
	script := `require "fileinto";
if header :contains ["X-Label", "Subject"] "message" { fileinto "HL"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "HL" {
		t.Errorf("expected match across a header list, got %q", got)
	}
}

// ── fileinto :create ─────────────────────────────────────────────────

func TestSieve_FileintoCreate(t *testing.T) {
	script := `require ["fileinto", "mailbox"];
if header :contains "Subject" "test" { fileinto :create "Archive/2026"; }`
	r := runSieve(t, script, sieveEmail())
	if folderOf(r) != "Archive/2026" {
		t.Errorf("expected folder Archive/2026, got %q", folderOf(r))
	}
	if r.meta["deliver_to_folder_create"] != "true" {
		t.Errorf("expected deliver_to_folder_create=true, got %q", r.meta["deliver_to_folder_create"])
	}
}

// ── imap4flags ───────────────────────────────────────────────────────

func TestSieve_SetAndAddFlag(t *testing.T) {
	script := `require "imap4flags";
if true {
  setflag "work";
  addflag "urgent";
  addflag "work";
}`
	r := runSieve(t, script, sieveEmail())
	if got := r.meta["imap_flags"]; got != "work urgent" {
		t.Errorf("expected flags 'work urgent' (deduped), got %q", got)
	}
}

func TestSieve_RemoveFlag(t *testing.T) {
	script := `require "imap4flags";
if true {
  setflag ["a", "b", "c"];
  removeflag "b";
}`
	r := runSieve(t, script, sieveEmail())
	if got := r.meta["imap_flags"]; got != "a c" {
		t.Errorf("expected flags 'a c' after removeflag, got %q", got)
	}
}

func TestSieve_SetFlagEscaped(t *testing.T) {
	// "\\Seen" in source is the IMAP system flag \Seen.
	script := `require "imap4flags";
if true { setflag "\\Seen"; }`
	r := runSieve(t, script, sieveEmail())
	if got := r.meta["imap_flags"]; got != `\Seen` {
		t.Errorf(`expected flag '\Seen', got %q`, got)
	}
}

// ── redirect with tag ────────────────────────────────────────────────

func TestSieve_RedirectCopyTag(t *testing.T) {
	script := `require ["copy"];
if true { redirect :copy "copy@example.com"; }`
	r := runSieve(t, script, sieveEmail())
	if got := r.meta["redirect_to"]; got != "copy@example.com" {
		t.Errorf("expected redirect_to copy@example.com, got %q", got)
	}
}

// ── stop across top-level commands ───────────────────────────────────

func TestSieve_TopLevelStop(t *testing.T) {
	script := `require "fileinto";
if true { fileinto "First"; }
stop;
if true { fileinto "Second"; }`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "First" {
		t.Errorf("expected top-level stop to halt, got %q", got)
	}
}

// ── multi-line string ────────────────────────────────────────────────

func TestSieve_MultiLineString(t *testing.T) {
	script := "require \"vacation\";\n" +
		"vacation :subject \"OOO\"\n" +
		"text:\n" +
		"Line one.\n" +
		"Line two.\n" +
		".\n" +
		";"
	r := runSieve(t, script, sieveEmail())
	body := r.meta["vacation_reply_body"]
	if !strings.Contains(body, "Line one.") || !strings.Contains(body, "Line two.") {
		t.Errorf("expected multi-line vacation body, got %q", body)
	}
}

func TestSieve_MultiLineDotStuffing(t *testing.T) {
	script := "require \"vacation\";\n" +
		"vacation :subject \"OOO\"\n" +
		"text:\n" +
		"..dotted line\n" +
		".\n" +
		";"
	r := runSieve(t, script, sieveEmail())
	body := r.meta["vacation_reply_body"]
	if !strings.Contains(body, ".dotted line") || strings.Contains(body, "..dotted") {
		t.Errorf("expected dot-unstuffed body, got %q", body)
	}
}

// ── comments and escapes ─────────────────────────────────────────────

func TestSieve_Comments(t *testing.T) {
	script := `require "fileinto";
# hash comment
/* bracket
   comment */
if header :contains "Subject" "test" { # trailing comment
  fileinto "Commented";
}`
	if got := folderOf(runSieve(t, script, sieveEmail())); got != "Commented" {
		t.Errorf("expected match despite comments, got %q", got)
	}
}

func TestSieve_EscapedQuoteInString(t *testing.T) {
	email := sieveEmail()
	email.Headers.Subject = `say "hi"`
	script := `require "fileinto";
if header :is "Subject" "say \"hi\"" { fileinto "Quoted"; }`
	if got := folderOf(runSieve(t, script, email)); got != "Quoted" {
		t.Errorf("expected escaped-quote string match, got %q", got)
	}
}

// ── extension leniency ───────────────────────────────────────────────

func TestSieve_UnknownCommandRejected(t *testing.T) {
	// An unknown command (from an undeclared extension, or a typo) must be a
	// parse error, not a silent no-op that lets the rest of the script run.
	script := `require "fileinto";
frobnicate "x" 42;
if header :contains "Subject" "test" { fileinto "StillWorks"; }`
	if err := Validate(script); err == nil {
		t.Error("expected an unknown command to be rejected at parse time")
	}
}

func TestSieve_UnknownTestRejected(t *testing.T) {
	// An unknown test must be a parse error, not silently treated as false.
	script := `require "fileinto";
if spamtest :value "gt" "5" {
  fileinto "Spam";
} else {
  fileinto "Ham";
}`
	if err := Validate(script); err == nil {
		t.Error("expected an unknown test to be rejected at parse time")
	}
}

// ── terminal dispositions ────────────────────────────────────────────

func TestSieve_Discard(t *testing.T) {
	script := `if header :is "Subject" "Test message" {
  discard;
}`
	if got := runSieve(t, script, sieveEmail()).disposition; got != Discard {
		t.Errorf("expected Discard, got %v", got)
	}
}

func TestSieve_Reject(t *testing.T) {
	script := `require "reject";
if header :is "Subject" "Test message" {
  reject "Not accepted";
}`
	r := runSieve(t, script, sieveEmail())
	if r.disposition != Reject {
		t.Errorf("expected Reject, got %v", r.disposition)
	}
	if r.reject != "Not accepted" {
		t.Errorf("expected reject reason 'Not accepted', got %q", r.reject)
	}
}

// ── header / redirect basics ─────────────────────────────────────────

func TestSieve_HeaderContains(t *testing.T) {
	script := `require "fileinto";
if header :contains "Subject" "Test" {
  fileinto "TestFolder";
}`
	r := runSieve(t, script, sieveEmail())
	if r.disposition != Continue {
		t.Fatalf("expected Continue, got %v", r.disposition)
	}
	if r.meta["deliver_to_folder"] != "TestFolder" {
		t.Errorf("expected deliver_to_folder=TestFolder, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_Redirect(t *testing.T) {
	script := `if header :contains "Subject" "Test" {
  redirect "other@example.com";
}`
	r := runSieve(t, script, sieveEmail())
	if r.disposition != Continue {
		t.Fatalf("expected Continue, got %v", r.disposition)
	}
	if r.meta["redirect_to"] != "other@example.com" {
		t.Errorf("expected redirect_to=other@example.com, got %q", r.meta["redirect_to"])
	}
}

// ── body extension ───────────────────────────────────────────────────

func TestSieve_BodyContains(t *testing.T) {
	script := `require ["body", "fileinto"];
if body :contains "test message" {
  fileinto "BodyMatch";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] != "BodyMatch" {
		t.Errorf("expected deliver_to_folder=BodyMatch, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_BodyContains_NoMatch(t *testing.T) {
	script := `require ["body", "fileinto"];
if body :contains "nonexistent phrase" {
  fileinto "ShouldNotMatch";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] == "ShouldNotMatch" {
		t.Error("body :contains should not have matched")
	}
}

func TestSieve_BodyContains_HTMLStripped(t *testing.T) {
	script := `require ["body", "fileinto"];
if body :contains "important" {
  fileinto "HTMLMatch";
}`
	email := sieveEmail()
	email.Body.ContentType = "text/html"
	email.Body.Content = "<html><body><b>This is important</b> content.</body></html>"

	r := runSieve(t, script, email)
	if r.meta["deliver_to_folder"] != "HTMLMatch" {
		t.Errorf("expected deliver_to_folder=HTMLMatch, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_BodyContains_MultipartPlainPreferred(t *testing.T) {
	script := `require ["body", "fileinto"];
if body :contains "plain text content" {
  fileinto "PlainMatch";
}`
	email := sieveEmail()
	email.Body.ContentType = "multipart/alternative"
	email.Body.Content = ""
	email.Body.Parts = []Body{
		{ContentType: "text/plain", Content: "This is plain text content."},
		{ContentType: "text/html", Content: "<p>This is HTML content.</p>"},
	}

	r := runSieve(t, script, email)
	if r.meta["deliver_to_folder"] != "PlainMatch" {
		t.Errorf("expected body match on text/plain part, got folder=%q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_BodyIs(t *testing.T) {
	script := `require ["body", "fileinto"];
if body :is "Hello, this is a test message body." {
  fileinto "ExactMatch";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] != "ExactMatch" {
		t.Errorf("expected deliver_to_folder=ExactMatch, got %q", r.meta["deliver_to_folder"])
	}
}

// ── regex extension ──────────────────────────────────────────────────

func TestSieve_HeaderRegex(t *testing.T) {
	script := `require ["regex", "fileinto"];
if header :regex "Subject" ".*[Tt]est.*" {
  fileinto "RegexMatch";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] != "RegexMatch" {
		t.Errorf("expected deliver_to_folder=RegexMatch, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_HeaderRegex_NoMatch(t *testing.T) {
	script := `require ["regex", "fileinto"];
if header :regex "Subject" "^URGENT:.*" {
  fileinto "UrgentFolder";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] == "UrgentFolder" {
		t.Error("regex should not have matched")
	}
}

func TestSieve_HeaderRegex_CaseInsensitive(t *testing.T) {
	script := `require ["regex", "fileinto"];
if header :regex "Subject" "^test message$" {
  fileinto "CaseInsensitive";
}`
	email := sieveEmail()
	email.Headers.Subject = "Test Message"
	r := runSieve(t, script, email)
	if r.meta["deliver_to_folder"] != "CaseInsensitive" {
		t.Errorf("expected case-insensitive regex match, got folder=%q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_BodyRegex(t *testing.T) {
	script := `require ["body", "regex", "fileinto"];
if body :regex "test.*body" {
  fileinto "BodyRegex";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] != "BodyRegex" {
		t.Errorf("expected deliver_to_folder=BodyRegex, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_AddressRegex(t *testing.T) {
	script := `require ["regex", "fileinto"];
if address :regex "From" "sender@.*\.com" {
  fileinto "AddressRegex";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] != "AddressRegex" {
		t.Errorf("expected deliver_to_folder=AddressRegex, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_InvalidRegex_Skipped(t *testing.T) {
	script := `require ["regex", "fileinto"];
if header :regex "Subject" "[invalid" {
  fileinto "ShouldNotMatch";
}`
	r := runSieve(t, script, sieveEmail())
	// Invalid regex should not cause an error, just not match.
	if r.meta["deliver_to_folder"] == "ShouldNotMatch" {
		t.Error("invalid regex should not match")
	}
}

// ── envelope extension ───────────────────────────────────────────────

func TestSieve_EnvelopeFrom_Is(t *testing.T) {
	script := `require ["envelope", "fileinto"];
if envelope :is "from" "sender@example.com" {
  fileinto "EnvelopeMatch";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] != "EnvelopeMatch" {
		t.Errorf("expected deliver_to_folder=EnvelopeMatch, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_EnvelopeTo_Is(t *testing.T) {
	script := `require ["envelope", "fileinto"];
if envelope :is "to" "recipient@example.com" {
  fileinto "EnvelopeTo";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] != "EnvelopeTo" {
		t.Errorf("expected deliver_to_folder=EnvelopeTo, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_EnvelopeFrom_Contains(t *testing.T) {
	script := `require ["envelope", "fileinto"];
if envelope :contains "from" "example.com" {
  fileinto "EnvelopeDomain";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] != "EnvelopeDomain" {
		t.Errorf("expected deliver_to_folder=EnvelopeDomain, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_EnvelopeFrom_NoMatch(t *testing.T) {
	script := `require ["envelope", "fileinto"];
if envelope :is "from" "other@example.com" {
  fileinto "ShouldNotMatch";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] == "ShouldNotMatch" {
		t.Error("envelope :is should not have matched")
	}
}

func TestSieve_EnvelopeFrom_NullReversePath(t *testing.T) {
	// RFC 5228 §5.4 / RFC 5231: a null reverse-path (SMTP "MAIL FROM:<>", a
	// bounce) has an empty-string envelope "from" value, so the canonical
	// bounce-detection idiom `envelope :is "from" ""` MUST match it.
	script := `require ["envelope", "fileinto"];
if envelope :is "from" "" {
  fileinto "Bounce";
}`
	email := sieveEmail()
	email.Envelope.From = ""
	email.Envelope.FromNull = true
	if got := folderOf(runSieve(t, script, email)); got != "Bounce" {
		t.Errorf("expected null reverse-path to match :is \"\", got %q", got)
	}
}

func TestSieve_EnvelopeFrom_AbsentDoesNotMatchEmpty(t *testing.T) {
	// A genuinely absent envelope sender (no MAIL FROM at all) is distinct from
	// a null reverse-path: it produces no value and must not match :is "".
	script := `require ["envelope", "fileinto"];
if envelope :is "from" "" {
  fileinto "ShouldNotMatch";
}`
	email := sieveEmail()
	email.Envelope.From = ""
	email.Envelope.FromNull = false
	if got := folderOf(runSieve(t, script, email)); got == "ShouldNotMatch" {
		t.Error("absent envelope sender must not match :is \"\"")
	}
}

func TestSieve_EnvelopeFrom_NullReversePath_NoFalseMatch(t *testing.T) {
	// A null reverse-path is the empty string only: it must not match a
	// non-empty key.
	script := `require ["envelope", "fileinto"];
if envelope :is "from" "someone@example.com" {
  fileinto "ShouldNotMatch";
}`
	email := sieveEmail()
	email.Envelope.From = ""
	email.Envelope.FromNull = true
	if got := folderOf(runSieve(t, script, email)); got == "ShouldNotMatch" {
		t.Error("null reverse-path must not match a non-empty key")
	}
}

func TestSieve_EnvelopeFrom_NullReversePath_Parts(t *testing.T) {
	// The :localpart and :domain of a null reverse-path are both the empty
	// string (RFC 5231), so each part matches :is "".
	for _, part := range []string{":localpart", ":domain"} {
		script := `require ["envelope", "fileinto"];
if envelope ` + part + ` :is "from" "" {
  fileinto "EmptyPart";
}`
		email := sieveEmail()
		email.Envelope.From = ""
		email.Envelope.FromNull = true
		if got := folderOf(runSieve(t, script, email)); got != "EmptyPart" {
			t.Errorf("expected %s of null reverse-path to match \"\", got %q", part, got)
		}
	}
}

func TestSieve_EnvelopeFrom_Resolved(t *testing.T) {
	// The host resolves any gateway-supplied envelope sender before evaluating.
	script := `require ["envelope", "fileinto"];
if envelope :is "from" "gateway-sender@example.com" {
  fileinto "MetadataEnvelope";
}`
	email := sieveEmail()
	email.Envelope.From = "gateway-sender@example.com"
	r := runSieve(t, script, email)
	if r.meta["deliver_to_folder"] != "MetadataEnvelope" {
		t.Errorf("expected deliver_to_folder=MetadataEnvelope, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_EnvelopeTo_Resolved(t *testing.T) {
	script := `require ["envelope", "fileinto"];
if envelope :is "to" "special-rcpt@example.com" {
  fileinto "MetadataEnvelopeTo";
}`
	email := sieveEmail()
	email.Envelope.To = []string{"special-rcpt@example.com"}
	r := runSieve(t, script, email)
	if r.meta["deliver_to_folder"] != "MetadataEnvelopeTo" {
		t.Errorf("expected deliver_to_folder=MetadataEnvelopeTo, got %q", r.meta["deliver_to_folder"])
	}
}

func TestSieve_EnvelopeRegex(t *testing.T) {
	script := `require ["envelope", "regex", "fileinto"];
if envelope :regex "from" ".*@example\.com" {
  fileinto "EnvelopeRegex";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] != "EnvelopeRegex" {
		t.Errorf("expected deliver_to_folder=EnvelopeRegex, got %q", r.meta["deliver_to_folder"])
	}
}

// ── vacation extension ───────────────────────────────────────────────

func TestSieve_Vacation_Basic(t *testing.T) {
	script := `require "vacation";
vacation :days 7 :subject "Out of Office" "I am currently on vacation.";`
	r := runSieve(t, script, sieveEmail())

	if r.disposition != Continue {
		t.Fatalf("expected Continue, got %v", r.disposition)
	}
	if r.meta["vacation_reply_to"] != "sender@example.com" {
		t.Errorf("expected vacation_reply_to=sender@example.com, got %q", r.meta["vacation_reply_to"])
	}
	if r.meta["vacation_reply_subject"] != "Out of Office" {
		t.Errorf("expected vacation_reply_subject='Out of Office', got %q", r.meta["vacation_reply_subject"])
	}
	if r.meta["vacation_reply_body"] != "I am currently on vacation." {
		t.Errorf("expected vacation_reply_body='I am currently on vacation.', got %q", r.meta["vacation_reply_body"])
	}
	if r.meta["vacation_days"] != "7" {
		t.Errorf("expected vacation_days=7, got %q", r.meta["vacation_days"])
	}
}

func TestSieve_Vacation_DefaultDays(t *testing.T) {
	script := `require "vacation";
vacation :subject "Away" "Gone fishing.";`
	r := runSieve(t, script, sieveEmail())
	if r.meta["vacation_days"] != "7" {
		t.Errorf("expected default vacation_days=7, got %q", r.meta["vacation_days"])
	}
}

func TestSieve_Vacation_Conditional(t *testing.T) {
	script := `require "vacation";
if header :contains "Subject" "Test" {
  vacation :days 3 :subject "Auto-reply" "Got your test message.";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["vacation_reply_body"] != "Got your test message." {
		t.Errorf("expected vacation body, got %q", r.meta["vacation_reply_body"])
	}
	if r.meta["vacation_days"] != "3" {
		t.Errorf("expected vacation_days=3, got %q", r.meta["vacation_days"])
	}
}

func TestSieve_Vacation_UsesEnvelopeSender(t *testing.T) {
	script := `require "vacation";
vacation :subject "Out" "Away.";`
	email := sieveEmail()
	email.Envelope.From = "envelope-sender@test.com"
	r := runSieve(t, script, email)
	if r.meta["vacation_reply_to"] != "envelope-sender@test.com" {
		t.Errorf("expected vacation_reply_to from envelope, got %q", r.meta["vacation_reply_to"])
	}
}

func TestSieve_Vacation_MultiLine(t *testing.T) {
	script := `require "vacation";
vacation :days 14
  :subject "On Leave"
  "I will be out of office until March 1st.";`
	r := runSieve(t, script, sieveEmail())
	if r.meta["vacation_reply_subject"] != "On Leave" {
		t.Errorf("expected subject 'On Leave', got %q", r.meta["vacation_reply_subject"])
	}
	if r.meta["vacation_reply_body"] != "I will be out of office until March 1st." {
		t.Errorf("expected body about March, got %q", r.meta["vacation_reply_body"])
	}
	if r.meta["vacation_days"] != "14" {
		t.Errorf("expected vacation_days=14, got %q", r.meta["vacation_days"])
	}
}

// ── notify extension ─────────────────────────────────────────────────

func TestSieve_Notify_Basic(t *testing.T) {
	script := `require "notify";
notify :method "mailto:admin@example.com" :message "New mail received";`
	r := runSieve(t, script, sieveEmail())

	if r.disposition != Continue {
		t.Fatalf("expected Continue, got %v", r.disposition)
	}
	if r.meta["notify_method"] != "mailto:admin@example.com" {
		t.Errorf("expected notify_method=mailto:admin@example.com, got %q", r.meta["notify_method"])
	}
	if r.meta["notify_message"] != "New mail received" {
		t.Errorf("expected notify_message='New mail received', got %q", r.meta["notify_message"])
	}
}

func TestSieve_Notify_Conditional(t *testing.T) {
	script := `require "notify";
if header :contains "Subject" "URGENT" {
  notify :method "mailto:oncall@example.com" :message "Urgent mail arrived";
}`
	email := sieveEmail()
	email.Headers.Subject = "URGENT: Server down"
	r := runSieve(t, script, email)
	if r.meta["notify_method"] != "mailto:oncall@example.com" {
		t.Errorf("expected notify for urgent, got %q", r.meta["notify_method"])
	}
}

func TestSieve_Notify_NoMatch(t *testing.T) {
	script := `require "notify";
if header :contains "Subject" "URGENT" {
  notify :method "mailto:oncall@example.com" :message "Urgent mail arrived";
}`
	r := runSieve(t, script, sieveEmail()) // Subject is "Test message", no match
	if r.meta["notify_method"] != "" {
		t.Error("notify should not have been set for non-matching condition")
	}
}

func TestSieve_Notify_MultiLine(t *testing.T) {
	script := `require "notify";
notify :method "mailto:alerts@example.com"
  :message "Alert: new message arrived";`
	r := runSieve(t, script, sieveEmail())
	if r.meta["notify_method"] != "mailto:alerts@example.com" {
		t.Errorf("expected notify_method from multi-line, got %q", r.meta["notify_method"])
	}
	if r.meta["notify_message"] != "Alert: new message arrived" {
		t.Errorf("expected notify_message from multi-line, got %q", r.meta["notify_message"])
	}
}

// ── combined extensions ──────────────────────────────────────────────

func TestSieve_CombinedExtensions(t *testing.T) {
	script := `require ["vacation", "notify", "body", "envelope", "fileinto"];
if body :contains "urgent" {
  notify :method "mailto:admin@example.com" :message "Urgent body detected";
}
if envelope :is "from" "vip@important.com" {
  fileinto "VIP";
}
vacation :days 5 :subject "Away" "On vacation.";`

	email := sieveEmail()
	email.Body.Content = "This is an urgent request."
	email.Envelope.From = "vip@important.com"

	r := runSieve(t, script, email)

	if r.meta["notify_method"] != "mailto:admin@example.com" {
		t.Errorf("expected notify from body match, got %q", r.meta["notify_method"])
	}
	if r.meta["deliver_to_folder"] != "VIP" {
		t.Errorf("expected fileinto VIP from envelope, got %q", r.meta["deliver_to_folder"])
	}
	if r.meta["vacation_reply_to"] != "vip@important.com" {
		t.Errorf("expected vacation_reply_to, got %q", r.meta["vacation_reply_to"])
	}
}

func TestSieve_Stop(t *testing.T) {
	script := `require ["body", "fileinto"];
if body :contains "test" {
  fileinto "First";
  stop;
}
if body :contains "test" {
  fileinto "Second";
}`
	r := runSieve(t, script, sieveEmail())
	if r.meta["deliver_to_folder"] != "First" {
		t.Errorf("expected stop to prevent second rule, got folder=%q", r.meta["deliver_to_folder"])
	}
}

// ── extractBodyText ──────────────────────────────────────────────────

func TestExtractBodyText_PlainText(t *testing.T) {
	msg := &Message{Body: Body{ContentType: "text/plain", Content: "Hello world"}}
	if got := extractBodyText(msg); got != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", got)
	}
}

func TestExtractBodyText_HTML(t *testing.T) {
	msg := &Message{Body: Body{ContentType: "text/html", Content: "<p>Hello <b>world</b></p>"}}
	got := extractBodyText(msg)
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "world") {
		t.Errorf("expected stripped HTML to contain 'Hello' and 'world', got %q", got)
	}
}

func TestExtractBodyText_Multipart(t *testing.T) {
	msg := &Message{Body: Body{
		ContentType: "multipart/alternative",
		Parts: []Body{
			{ContentType: "text/plain", Content: "Plain version"},
			{ContentType: "text/html", Content: "<p>HTML version</p>"},
		},
	}}
	if got := extractBodyText(msg); got != "Plain version" {
		t.Errorf("expected 'Plain version' (prefer text/plain), got %q", got)
	}
}

func TestExtractBodyText_MultipartHTMLOnly(t *testing.T) {
	msg := &Message{Body: Body{
		ContentType: "multipart/alternative",
		Parts:       []Body{{ContentType: "text/html", Content: "<p>Only HTML</p>"}},
	}}
	got := extractBodyText(msg)
	if !strings.Contains(got, "Only HTML") {
		t.Errorf("expected stripped HTML content, got %q", got)
	}
}

// ── stripHTMLTags ────────────────────────────────────────────────────

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		input    string
		contains string
	}{
		{"<p>Hello</p>", "Hello"},
		{"<b>Bold</b> and <i>italic</i>", "Bold"},
		{"No tags at all", "No tags at all"},
		{"<div><span>Nested</span></div>", "Nested"},
	}
	for _, tc := range tests {
		got := stripHTMLTags(tc.input)
		if !strings.Contains(got, tc.contains) {
			t.Errorf("stripHTMLTags(%q) = %q, expected to contain %q", tc.input, got, tc.contains)
		}
	}
}
