package sieve

import (
	"fmt"
	"strings"
	"testing"
)

// TestBody_ContentTypeASCIICase checks that MIME selection uses ASCII case
// rules, independently of the comparator used on body content (RFC 5173 §5.2;
// RFC 2045 §§2 and 5.1). Unicode look-alikes cannot select an ASCII MIME type.
//
// checklist: BODY-19
func TestBody_ContentTypeASCIICase(t *testing.T) {
	cases := []struct {
		spec, part string
		want       bool
	}{
		{"TEXT/PLAIN", "text/plain", true},
		{"text/plain", "TEXT/PLAIN; charset=UTF-8", true},
		{"IMAGE", "image/png", true},
		{"image/pkix-cert", "IMAGE/PKIX-CERT", true},
		{"text/plaİn", "text/plain", false},
		{"İMAGE", "image/png", false},
		{"image/pKix-cert", "image/pkix-cert", false},
		{"text/plain", "text/plaİn", false},
		{"image", "İMAGE/png", false},
		{"image/pkix-cert", "image/pKix-cert", false},
		{"text/plаin", "text/plain", false}, // Cyrillic a
		{"text/pla\xffin", "text/plain", false},
	}
	for i, tc := range cases {
		for _, comparator := range []string{"i;octet", "i;ascii-casemap"} {
			t.Run(fmt.Sprintf("%d/%s", i, comparator), func(t *testing.T) {
				script := `require "body"; if body :content "` + tc.spec + `" :comparator "` + comparator + `" :is "needle" { discard; }`
				for _, nested := range []bool{false, true} {
					body := Body{ContentType: tc.part, Content: "needle"}
					if nested {
						body = Body{ContentType: "multipart/mixed", Parts: []Body{
							{ContentType: "message/rfc822", Parts: []Body{body}},
						}}
					}
					if got := bodyMatches(t, script, &Message{Body: body}); got != tc.want {
						t.Errorf("spec %q, part %q, nested %v: matched %v, want %v", tc.spec, tc.part, nested, got, tc.want)
					}
				}
			})
		}
	}
	// Encoded-character processing must not turn Unicode case folding into an
	// alternate route to an ASCII media type. Lists still select a valid entry.
	for _, tc := range []struct {
		spec string
		want bool
	}{
		{`"text/pla${unicode:0130}n"`, false},
		{`"text/pla${hex:49}n"`, true},
		{`["text/pla${unicode:0130}n", "text/plain"]`, true},
		{`["text/pla${unicode:0130}n", "image/png"]`, false},
	} {
		script := `require ["body", "encoded-character"]; if body :content ` + tc.spec + ` :is "needle" { discard; }`
		if got := bodyMatches(t, script, &Message{Body: Body{ContentType: "text/plain", Content: "needle"}}); got != tc.want {
			t.Errorf("encoded/list %s: matched %v, want %v", tc.spec, got, tc.want)
		}
	}
}

// FuzzBodyContentTypeSelection compares arbitrary selectors against known MIME
// types. The reference rejects non-ASCII selectors before using EqualFold;
// it does not use the evaluator's normalization or selection helpers.
//
// checklist: BODY-09 BODY-19
func FuzzBodyContentTypeSelection(f *testing.F) {
	for _, s := range []string{"", "text", "TEXT/PLAIN", "text/plaİn", "İmage", "image/pKix-cert", "text/", "/plain", "text/plain/x", "text/plain; charset=utf-8", " text/plain", "text/pla\xffin"} {
		for i := byte(0); i < 3; i++ {
			f.Add(s, i)
		}
	}
	f.Fuzz(func(t *testing.T, spec string, choice byte) {
		types := []string{"text/plain", "image/png", "image/pkix-cert"}
		mediaType := types[int(choice)%len(types)]
		typ, _, _ := strings.Cut(mediaType, "/")
		ascii := true
		for _, r := range spec {
			if r > 127 {
				ascii = false
				break
			}
		}
		want := ascii && (spec == "" || strings.EqualFold(spec, typ) || strings.EqualFold(spec, mediaType))
		for _, header := range []string{mediaType, strings.ToUpper(mediaType) + "; charset=utf-8"} {
			if got := contentTypeMatches(spec, header); got != want {
				t.Fatalf("selector %q, header %q: got %v, want %v", spec, header, got, want)
			}
		}
	})
}
