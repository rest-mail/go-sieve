package sieve

import "testing"

// These tests check RFC 5173 (the "body" extension) clause by clause. The host
// supplies each MIME part decoded, as a Body; the library selects the parts a
// body transform names and matches them.

// bodyMatches parses script, which must discard when its body test matches,
// and reports whether it did for msg.
func bodyMatches(t *testing.T, script string, msg *Message) bool {
	t.Helper()
	s, err := Parse(script)
	if err != nil {
		t.Fatalf("Parse(%q): %v", script, err)
	}
	return s.Evaluate(msg, newRecExec()).Disposition == Discard
}

// rfc5173Example is the example message of RFC 5173 §5.2, mapped onto Body the
// way the Content doc asks: a multipart part holds its prologue and epilogue,
// and the message/rfc822 part holds the nested header.
func rfc5173Example() *Message {
	return &Message{Body: Body{
		ContentType: "multipart/mixed; boundary=outer",
		Content:     "This is a multi-part message in MIME format.\r\n\r\nThis is the end of the outer MIME multipart.\r\n",
		Parts: []Body{
			{
				ContentType: "multipart/alternative; boundary=inner",
				Content:     "This is a nested multi-part message in MIME format.\r\n\r\nThis is the end of the inner MIME multipart.\r\n",
				Parts: []Body{
					{ContentType: `text/plain; charset="us-ascii"`, Content: "Hello\r\n"},
					{ContentType: `text/html; charset="us-ascii"`, Content: "<html><body>Hello</body></html>\r\n"},
				},
			},
			{
				ContentType: "message/rfc822",
				Content:     "From: Someone Else\r\nSubject: hello request\r\n",
				Parts:       []Body{{ContentType: "text/plain", Content: "Please say Hello\r\n"}},
			},
		},
	}}
}

// TestBody_RequiresCapability checks that the body test is an error unless the
// script declares "body" (RFC 5173 §3; RFC 5228 §3.2).
//
// checklist: BODY-01
func TestBody_RequiresCapability(t *testing.T) {
	if err := Validate(`require "body"; if body :contains "x" { discard; }`); err != nil {
		t.Errorf("with require: %v", err)
	}
	if err := Validate(`if body :contains "x" { discard; }`); err == nil {
		t.Error("accepted without require \"body\", want an error")
	}
}

// TestBody_ArgumentsAnyOrderAndDefaults checks that the comparator, match type
// and body transform may come in any order, and that the defaults are :is and
// i;ascii-casemap (RFC 5173 §4; RFC 5228 §2.6.2, §2.6.3).
//
// checklist: BODY-02
func TestBody_ArgumentsAnyOrderAndDefaults(t *testing.T) {
	msg := &Message{Body: Body{ContentType: "text/plain", Content: "Hello World"}}
	matching := []string{
		`require "body"; if body :content "text" :comparator "i;octet" :contains "World" { discard; }`,
		`require "body"; if body :contains :content "text" :comparator "i;octet" "World" { discard; }`,
		`require "body"; if body :comparator "i;octet" :contains :content "text" "World" { discard; }`,
		// Defaults: :is, compared case-insensitively.
		`require "body"; if body "hello world" { discard; }`,
	}
	for _, s := range matching {
		if !bodyMatches(t, s, msg) {
			t.Errorf("%q: no match, want one", s)
		}
	}
	// :is is the default, so a substring does not match.
	if bodyMatches(t, `require "body"; if body "Hello" { discard; }`, msg) {
		t.Error("default match type matched a substring, want :is")
	}
}

// TestBody_TransformSyntax checks that a body test takes at most one body
// transform, that :content takes a string list, and that transforms belong to
// the body test only (RFC 5173 §5).
//
// checklist: BODY-03
func TestBody_TransformSyntax(t *testing.T) {
	good := []string{
		`require "body"; if body :raw :contains "x" { discard; }`,
		`require "body"; if body :text :contains "x" { discard; }`,
		`require "body"; if body :content "text" :contains "x" { discard; }`,
		`require "body"; if body :content ["text/plain", "text/html"] :contains "x" { discard; }`,
	}
	for _, s := range good {
		if err := Validate(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	bad := []string{
		`require "body"; if body :raw :text :contains "x" { discard; }`,
		`require "body"; if body :content "text" :content "image" :contains "x" { discard; }`,
		`require "body"; if body :content :contains "x" { discard; }`,
		`require "body"; if header :raw :contains "subject" "x" { discard; }`,
	}
	for _, s := range bad {
		if err := Validate(s); err == nil {
			t.Errorf("%q: accepted, want an error", s)
		}
	}
}

// TestBody_DefaultTransformIsText checks that a body test without a transform
// behaves as :text (RFC 5173 §5).
//
// checklist: BODY-04 BODY-16
func TestBody_DefaultTransformIsText(t *testing.T) {
	msg := &Message{Body: Body{ContentType: "text/html", Content: "<b>bold</b> move"}}
	for _, s := range []string{"", ":text "} {
		if !bodyMatches(t, `require "body"; if body `+s+`:contains "bold" { discard; }`, msg) {
			t.Errorf("%q: the text did not match", s)
		}
		if bodyMatches(t, `require "body"; if body `+s+`:contains "<b>" { discard; }`, msg) {
			t.Errorf("%q: matched the mark-up, want the stripped text of :text", s)
		}
	}
}

// TestBody_NotLineByLine checks that the body is matched as a whole, CRLFs
// included, rather than line by line (RFC 5173 §4).
//
// checklist: BODY-06
func TestBody_NotLineByLine(t *testing.T) {
	msg := &Message{Body: Body{ContentType: "text/plain", Content: "first line\r\nsecond line\r\n"}}
	if !bodyMatches(t, `require "body"; if body :matches "first*second*" { discard; }`, msg) {
		t.Error(":matches did not span lines")
	}
	if bodyMatches(t, `require "body"; if body :is "second line" { discard; }`, msg) {
		t.Error(":is matched a single line, want the whole body")
	}
}

// TestBody_NoBodyIsFalse checks that a message which is only a header, with
// no empty line after it, has no body: every body test is false, even one for
// the empty string, while a message with an empty body still matches it (RFC
// 5173 §4). Such a message also has no blank line to count in its size.
//
// checklist: BODY-05
func TestBody_NoBodyIsFalse(t *testing.T) {
	scripts := []string{
		`require "body"; if body :is "" { discard; }`,
		`require "body"; if body :matches "*" { discard; }`,
		`require "body"; if body :contains "" { discard; }`,
		`require "body"; if body :raw :is "" { discard; }`,
		`require "body"; if body :content "" :contains "" { discard; }`,
		`require "body"; if not body :contains "x" { discard; }`,
	}
	headerOnly := &Message{Headers: Headers{Subject: "hi"}, NoBody: true}
	emptyBody := &Message{Headers: Headers{Subject: "hi"}}
	for i, s := range scripts {
		last := i == len(scripts)-1
		if got := bodyMatches(t, s, headerOnly); got != last {
			t.Errorf("%q on a message without a body: matched %v, want %v", s, got, last)
		}
		if !bodyMatches(t, s, emptyBody) {
			t.Errorf("%q on a message with an empty body: did not match", s)
		}
	}
	if got, want := messageSize(headerOnly), messageSize(emptyBody)-2; got != want {
		t.Errorf("size of a message without a body = %d, want %d (no blank line)", got, want)
	}
}

// TestBody_ContentTypeRules checks how a :content type selects parts: the
// empty string matches all, a bare type matches its subtypes, type/subtype
// matches itself, and a type with a leading, trailing or second slash matches
// nothing (RFC 5173 §5.2).
//
// checklist: BODY-09
func TestBody_ContentTypeRules(t *testing.T) {
	cases := []struct {
		spec, contentType string
		want              bool
	}{
		{"", "image/png", true},
		{"text", "text/plain", true},
		{"text", "TEXT/HTML; charset=utf-8", true},
		{"TEXT/Plain", "text/plain; charset=us-ascii", true},
		{"text/plain", "text/html", false},
		{"text", "texts/plain", false},
		{"text", "", true}, // no Content-Type is text/plain (RFC 2045 §5.2)
		{"/plain", "text/plain", false},
		{"text/", "text/plain", false},
		{"text/plain/x", "text/plain", false},
		{"/", "text/plain", false},
	}
	for _, c := range cases {
		if got := contentTypeMatches(c.spec, c.contentType); got != c.want {
			t.Errorf("contentTypeMatches(%q, %q) = %v, want %v", c.spec, c.contentType, got, c.want)
		}
	}
	msg := &Message{Body: Body{
		ContentType: "multipart/mixed",
		Parts: []Body{
			{ContentType: "text/plain", Content: "plain words"},
			{ContentType: "image/png", Content: "PNGDATA"},
		},
	}}
	if bodyMatches(t, `require "body"; if body :content "text/" :contains "plain" { discard; }`, msg) {
		t.Error(`:content "text/" matched, want no part`)
	}
	if !bodyMatches(t, `require "body"; if body :content "image/png" :contains "" { discard; }`, msg) {
		t.Error(`:content "image/png" :contains "" did not find the image part`)
	}
	if bodyMatches(t, `require "body"; if body :content "text" :contains "PNGDATA" { discard; }`, msg) {
		t.Error(`:content "text" searched the image part`)
	}
}

// TestBody_ContentRecursive checks that :content descends into multipart and
// message/rfc822 parts, searches every part of a matching type, and matches if
// any part matches any key (RFC 5173 §5.2).
//
// checklist: BODY-10
func TestBody_ContentRecursive(t *testing.T) {
	msg := &Message{Body: Body{
		ContentType: "multipart/mixed",
		Parts: []Body{
			{ContentType: "text/plain", Content: "first"},
			{ContentType: "message/rfc822", Content: "Subject: fwd\r\n", Parts: []Body{
				{ContentType: "multipart/alternative", Parts: []Body{
					{ContentType: "text/plain", Content: "deeply nested"},
				}},
			}},
		},
	}}
	if !bodyMatches(t, `require "body"; if body :content "text/plain" :contains "nested" { discard; }`, msg) {
		t.Error("did not find the text/plain part nested in message/rfc822")
	}
	if !bodyMatches(t, `require "body"; if body :content "text/plain" :contains ["nomatch", "first"] { discard; }`, msg) {
		t.Error("did not match the second key against the first part")
	}
}

// TestBody_NoMatchAcrossParts checks that each part is matched on its own: a
// key never spans two parts, and a matching container contributes only its own
// content, not that of its parts (RFC 5173 §5.2).
//
// checklist: BODY-12
func TestBody_NoMatchAcrossParts(t *testing.T) {
	msg := &Message{Body: Body{
		ContentType: "multipart/mixed",
		Content:     "prologue",
		Parts: []Body{
			{ContentType: "text/plain", Content: "foo"},
			{ContentType: "text/plain", Content: "bar"},
		},
	}}
	if bodyMatches(t, `require "body"; if body :content "text" :contains "foobar" { discard; }`, msg) {
		t.Error("a key matched across two parts")
	}
	if bodyMatches(t, `require "body"; if body :content "multipart" :contains "foo" { discard; }`, msg) {
		t.Error(":content \"multipart\" searched the nested parts")
	}
	if !bodyMatches(t, `require "body"; if body :content "multipart" :contains "prologue" { discard; }`, msg) {
		t.Error(":content \"multipart\" did not search the container's own content")
	}
}

// TestBody_NULDoesNotTerminate checks that a NUL octet does not end the content
// being compared (RFC 5173 §5.2).
//
// checklist: BODY-14
func TestBody_NULDoesNotTerminate(t *testing.T) {
	msg := &Message{Body: Body{ContentType: "text/plain", Content: "before\x00after"}}
	for _, s := range []string{
		`require "body"; if body :contains "after" { discard; }`,
		`require "body"; if body :content "text" :contains "after" { discard; }`,
	} {
		if !bodyMatches(t, s, msg) {
			t.Errorf("%q: text after a NUL was not searched", s)
		}
	}
}

// TestBody_TextKeepsPartTogether checks that :text puts all the text of one
// non-container part in the same string (RFC 5173 §5.3).
//
// checklist: BODY-15
func TestBody_TextKeepsPartTogether(t *testing.T) {
	msg := &Message{Body: Body{
		ContentType: "multipart/alternative",
		Parts:       []Body{{ContentType: "text/plain", Content: "project\r\nschedule"}},
	}}
	if !bodyMatches(t, `require "body"; if body :text :matches "project*schedule" { discard; }`, msg) {
		t.Error(":text split one part's text")
	}
}

// TestBody_RFCExample runs the tests RFC 5173 §5.2 describes against its
// example message, and checks that the example scripts of §5.1 to §5.3 parse.
//
// checklist: BODY-18 BODY-10 BODY-12
func TestBody_RFCExample(t *testing.T) {
	msg := rfc5173Example()
	cases := []struct {
		content, key string
		want         bool
	}{
		{"multipart", "MIME", true},
		{"multipart", "Hello", false},
		{"text/plain", "Hello", true},
		{"text/plain", "<html>", false},
		{"text/html", "<body>Hello", true},
		{"text", "Please say Hello", true},
		{"message/rfc822", "hello request", true},
		{"message/rfc822", "Please say", false},
	}
	for _, c := range cases {
		s := `require "body"; if body :content "` + c.content + `" :contains "` + c.key + `" { discard; }`
		if got := bodyMatches(t, s, msg); got != c.want {
			t.Errorf("%q = %v, want %v", s, got, c.want)
		}
	}
	for _, s := range []string{
		`require "body"; if body :raw :contains "MAKE MONEY FAST" { discard; }`,
		`require ["body", "fileinto"]; if body :content "text" :contains ["missile", "coordinates"] { fileinto "secrets"; } if body :content "audio/mp3" :contains "" { fileinto "jukebox"; }`,
		`require ["body", "fileinto"]; if body :text :contains "project schedule" { fileinto "project/schedule"; }`,
	} {
		if err := Validate(s); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
}
