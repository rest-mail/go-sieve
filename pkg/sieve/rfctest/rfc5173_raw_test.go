package rfctest

import (
	"errors"
	"strings"
	"testing"

	"github.com/rest-mail/go-sieve/pkg/sieve"
)

// RFC 5173 §5.1: ":raw" matches the entire undecoded body as one item. The
// MIME structure is irrelevant: transfer encodings stay, and multipart
// boundaries and the MIME headers of enclosed parts are content. The body is
// everything after the empty line that ends the header (§4).

// rfc5173RawExample is the body of the RFC 5173 §5.2 example message, after
// the header and its empty line, as it is on the wire.
const rfc5173RawExample = "This is a multi-part message in MIME format.\r\n" +
	"\r\n" +
	"--outer\r\n" +
	"Content-Type: multipart/alternative; boundary=inner\r\n" +
	"\r\n" +
	"This is a nested multi-part message in MIME format.\r\n" +
	"\r\n" +
	"--inner\r\n" +
	"Content-Type: text/plain; charset=\"us-ascii\"\r\n" +
	"\r\n" +
	"Hello\r\n" +
	"\r\n" +
	"--inner\r\n" +
	"Content-Type: text/html; charset=\"us-ascii\"\r\n" +
	"\r\n" +
	"<html><body>Hello</body></html>\r\n" +
	"\r\n" +
	"--inner--\r\n" +
	"\r\n" +
	"This is the end of the inner MIME multipart.\r\n" +
	"\r\n" +
	"--outer\r\n" +
	"Content-Type: message/rfc822\r\n" +
	"\r\n" +
	"From: Someone Else\r\n" +
	"Subject: hello request\r\n" +
	"\r\n" +
	"Please say Hello\r\n" +
	"\r\n" +
	"--outer--\r\n" +
	"\r\n" +
	"This is the end of the outer MIME multipart.\r\n"

// rfc5173ExampleMessage maps the §5.2 example onto Message: the decoded parts
// in Body, the wire body in RawBody, the outer header in Headers.
func rfc5173ExampleMessage() *sieve.Message {
	return &sieve.Message{
		Headers: sieve.Headers{
			Subject: "whatever",
			Raw:     map[string][]string{"Content-Type": {"multipart/mixed; boundary=outer"}},
		},
		Body: sieve.Body{
			ContentType: "multipart/mixed; boundary=outer",
			Content:     "This is a multi-part message in MIME format.\r\n\r\nThis is the end of the outer MIME multipart.\r\n",
			Parts: []sieve.Body{
				{
					ContentType: "multipart/alternative; boundary=inner",
					Content:     "This is a nested multi-part message in MIME format.\r\n\r\nThis is the end of the inner MIME multipart.\r\n",
					Parts: []sieve.Body{
						{ContentType: `text/plain; charset="us-ascii"`, Content: "Hello\r\n"},
						{ContentType: `text/html; charset="us-ascii"`, Content: "<html><body>Hello</body></html>\r\n"},
					},
				},
				{
					ContentType: "message/rfc822",
					Content:     "From: Someone Else\r\nSubject: hello request\r\n",
					Parts:       []sieve.Body{{ContentType: "text/plain", Content: "Please say Hello\r\n"}},
				},
			},
		},
		RawBody:    rfc5173RawExample,
		RawBodySet: true,
	}
}

// encodedMessage is a multipart message with a base64 and a quoted-printable
// part. Body holds the decoded text a host extracts; RawBody the wire form.
func encodedMessage() *sieve.Message {
	raw := "--b1\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: base64\r\n" +
		"\r\n" +
		"TUFLRSBNT05FWSBGQVNU\r\n" + // "MAKE MONEY FAST"
		"--b1\r\n" +
		"Content-Type: text/html; charset=utf-8\r\n" +
		"Content-Transfer-Encoding: quoted-printable\r\n" +
		"\r\n" +
		"<p class=3D\"x\">caf=C3=A9</p>\r\n" +
		"--b1--\r\n"
	return &sieve.Message{
		Body: sieve.Body{
			ContentType: "multipart/alternative; boundary=b1",
			Parts: []sieve.Body{
				{ContentType: "text/plain; charset=utf-8", Content: "MAKE MONEY FAST"},
				{ContentType: "text/html; charset=utf-8", Content: `<p class="x">café</p>`},
			},
		},
		RawBody:    raw,
		RawBodySet: true,
	}
}

// bodyDiscards evaluates script and reports whether it discarded, failing the
// test on a runtime error.
func bodyDiscards(t *testing.T, script string, msg *sieve.Message) bool {
	t.Helper()
	s, err := sieve.Parse(script)
	if err != nil {
		t.Fatalf("Parse(%q): %v", script, err)
	}
	out := s.Evaluate(msg, newRecExec())
	if out.Error != nil {
		t.Fatalf("%q: runtime error %v", script, out.Error)
	}
	return out.Disposition == sieve.Discard
}

// checklist: BODY-07 BODY-18
func TestBody_RawUndecoded(t *testing.T) {
	tests := []struct {
		name string
		msg  func() *sieve.Message
		test string
		want bool
	}{
		// §5.1 example: the literal text in MIME headers other than the outer
		// header matches. Here it is in a nested part's header.
		{"RFC example, text in a nested MIME header", func() *sieve.Message {
			m := rfc5173ExampleMessage()
			m.RawBody = "--outer\r\nX-Note: MAKE MONEY FAST\r\n\r\nhi\r\n--outer--\r\n"
			return m
		}, `body :raw :contains "MAKE MONEY FAST"`, true},
		{"RFC example, outer header is not body", func() *sieve.Message {
			m := rfc5173ExampleMessage()
			m.Headers.Subject = "MAKE MONEY FAST"
			return m
		}, `body :raw :contains "MAKE MONEY FAST"`, false},
		{"outer boundary line", rfc5173ExampleMessage, "body :raw :contains \"--outer\r\nContent-Type: message/rfc822\r\n\"", true},
		{"closing boundaries", rfc5173ExampleMessage, `body :raw :matches "*--inner--*--outer--*"`, true},
		{"nested MIME header", rfc5173ExampleMessage, `body :raw :contains "Content-Type: text/html; charset=\"us-ascii\""`, true},
		{"nested message header", rfc5173ExampleMessage, `body :raw :contains "Subject: hello request"`, true},
		{"prologue and epilogue", rfc5173ExampleMessage, "body :raw :matches \"This is a multi-part*This is the end of the outer MIME multipart.\r\n\"", true},
		{"one string, across parts", rfc5173ExampleMessage, `body :raw :matches "*Hello*Please say Hello*"`, true},
		{"the whole body with :is", rfc5173ExampleMessage, `body :raw :is "` + strings.ReplaceAll(rfc5173RawExample, `"`, `\"`) + `"`, true},
		{"one part alone is not the whole body", rfc5173ExampleMessage, "body :raw :is \"Hello\r\n\"", false},
		// §5.1: no transfer encoding is removed.
		{"base64 stays encoded", encodedMessage, `body :raw :contains "TUFLRSBNT05FWSBGQVNU"`, true},
		{"base64 is not decoded", encodedMessage, `body :raw :contains "MAKE MONEY FAST"`, false},
		{"quoted-printable stays encoded", encodedMessage, `body :raw :contains "class=3D\"x\">caf=C3=A9"`, true},
		{"quoted-printable is not decoded", encodedMessage, `body :raw :contains "café"`, false},
		{"Content-Transfer-Encoding header", encodedMessage, `body :raw :contains "Content-Transfer-Encoding: base64"`, true},
		{"HTML mark-up is not stripped", encodedMessage, `body :raw :contains "<p class="`, true},
		{"comparator applies to raw octets", encodedMessage, `body :raw :comparator "i;octet" :contains "tuflrsbnt05fwsbgqvnu"`, false},
		{"default comparator is case-insensitive", encodedMessage, `body :raw :contains "tuflrsbnt05fwsbgqvnu"`, true},
		{"8-bit and NUL octets", func() *sieve.Message {
			return &sieve.Message{RawBody: "a\x00b\xffc", RawBodySet: true}
		}, "body :raw :comparator \"i;octet\" :contains \"b\xffc\"", true},
		{"a supplied empty body", func() *sieve.Message {
			return &sieve.Message{RawBodySet: true}
		}, `body :raw :is ""`, true},
		// §4: a message with no body fails every body test.
		{"no body at all", func() *sieve.Message {
			return &sieve.Message{RawBodySet: true, NoBody: true}
		}, `body :raw :is ""`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := `require "body"; if ` + tc.test + ` { discard; }`
			if got := bodyDiscards(t, s, tc.msg()); got != tc.want {
				t.Errorf("%s = %v, want %v", tc.test, got, tc.want)
			}
		})
	}
}

// TestBody_RawIndependentOfOtherTransforms checks that :text and :content
// keep matching the decoded Body while :raw matches RawBody (§5.1–§5.3).
//
// checklist: BODY-07
func TestBody_RawIndependentOfOtherTransforms(t *testing.T) {
	tests := []struct {
		test string
		want bool
	}{
		{`body :text :contains "MAKE MONEY FAST"`, true},
		{`body :raw :contains "MAKE MONEY FAST"`, false},
		{`body :content "text/html" :contains "café"`, true},
		{`body :content "text/html" :contains "=C3=A9"`, false},
		{`body :content "text" :contains "--b1"`, false},
		{`body :text :contains "Content-Transfer-Encoding"`, false},
		{`body :raw :contains "--b1"`, true},
	}
	for _, tc := range tests {
		s := `require "body"; if ` + tc.test + ` { discard; }`
		if got := bodyDiscards(t, s, encodedMessage()); got != tc.want {
			t.Errorf("%s = %v, want %v", tc.test, got, tc.want)
		}
	}
	// The decoded tests do not need the raw body.
	m := encodedMessage()
	m.RawBody, m.RawBodySet = "", false
	if !bodyDiscards(t, `require "body"; if body :content "text/plain" :is "MAKE MONEY FAST" { discard; }`, m) {
		t.Error(":content needed the raw body")
	}
}

// TestBody_RawUnavailable checks the defined behaviour for a host that does
// not supply the undecoded body: reaching body :raw is a runtime error that
// fails safe to the implicit keep (RFC 5228 §2.10.6), never a match against
// decoded text, which §5.1 forbids. A :raw test that is not evaluated, or a
// message without a body, raises nothing.
//
// checklist: BODY-07
func TestBody_RawUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		msg     *sieve.Message
		wantErr bool
		applied []string
	}{
		{
			name:    "raw test reached",
			script:  `require ["body", "fileinto"]; fileinto "Before"; if body :raw :contains "Hello" { discard; } fileinto "After";`,
			msg:     &sieve.Message{Body: sieve.Body{ContentType: "text/plain", Content: "Hello"}},
			wantErr: true,
			applied: []string{"fileinto:Before"},
		},
		{
			name:    "negated raw test reached",
			script:  `require "body"; if not body :raw :contains "Hello" { discard; }`,
			msg:     &sieve.Message{Body: sieve.Body{ContentType: "text/plain", Content: "x"}},
			wantErr: true,
		},
		{
			name:    "raw test short-circuited",
			script:  `require "body"; if anyof (true, body :raw :contains "Hello") { discard; }`,
			msg:     &sieve.Message{},
			applied: nil,
		},
		{
			name:   "no body",
			script: `require "body"; if body :raw :contains "" { discard; }`,
			msg:    &sieve.Message{NoBody: true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := sieve.Parse(tc.script)
			if err != nil {
				t.Fatal(err)
			}
			ex := newRecExec()
			out := s.Evaluate(tc.msg, ex)
			if got := errors.Is(out.Error, sieve.ErrRawBodyUnavailable); got != tc.wantErr {
				t.Fatalf("Outcome.Error = %v, want ErrRawBodyUnavailable %v", out.Error, tc.wantErr)
			}
			if tc.wantErr && (!out.ImplicitKeep || out.Disposition != sieve.Continue) {
				t.Errorf("runtime error did not fail safe to keep: %+v", out)
			}
			if tc.wantErr && !equalStrings(ex.applied, tc.applied) {
				t.Errorf("applied = %v, want %v", ex.applied, tc.applied)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
