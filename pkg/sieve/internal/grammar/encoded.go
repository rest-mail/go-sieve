package grammar

import (
	"fmt"
	"strconv"
	"strings"
)

// DecodeEncodedCharacters applies the "encoded-character" transformation to s.
// Within a string, "${hex:HH HH ...}" is replaced by the octets named by its
// blank-separated 1*2-hex-digit groups, and "${unicode:XXXX ...}" by the UTF-8
// encoding of its blank-separated code points. The keywords are matched
// case-insensitively. A "${...}" run that does not match the grammar is left
// exactly as written (not an error); a well-formed ${unicode:...} whose value is
// a surrogate (U+D800..U+DFFF) or exceeds U+10FFFF is an error. The transform is
// a single left-to-right pass — its own output is never re-scanned, so at most
// one transformation is applied to any position (RFC 5228 §2.4.2.4).
func DecodeEncodedCharacters(s string) (string, error) {
	// Fast path: without a "${" nothing can match, so the string is unchanged.
	if !strings.Contains(s, "${") {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '$' && i+1 < len(s) && s[i+1] == '{' {
			decoded, end, err := decodeEncodedSequence(s, i)
			if err != nil {
				return "", err
			}
			if end > i {
				b.WriteString(decoded)
				i = end
				continue
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String(), nil
}

// decodeEncodedSequence tries to decode a single "${hex:...}" or "${unicode:...}"
// sequence beginning at start, where s[start:start+2] is "${". On a grammar match
// it returns the decoded octets and the index just past the closing "}"; when the
// run does not match the grammar it returns end == start so the caller emits the
// "$" literally and resumes at the following octet. A well-formed but out-of-range
// ${unicode:...} value returns an error.
func decodeEncodedSequence(s string, start int) (decoded string, end int, err error) {
	body := s[start+2:] // text after "${"
	var payloadStart int
	isUnicode := false
	switch {
	case hasASCIIFoldPrefix(body, "hex:"):
		payloadStart = start + 2 + len("hex:")
	case hasASCIIFoldPrefix(body, "unicode:"):
		payloadStart = start + 2 + len("unicode:")
		isUnicode = true
	default:
		return "", start, nil // unknown encoding keyword: leave literal
	}
	// The payload contains only hex digits and blanks, so the first "}" (if any)
	// terminates the sequence.
	rel := strings.IndexByte(s[payloadStart:], '}')
	if rel < 0 {
		return "", start, nil // no closing brace: leave literal
	}
	payload := s[payloadStart : payloadStart+rel]
	if isUnicode {
		out, ok, verr := decodeUnicodeSeq(payload)
		if verr != nil {
			return "", 0, verr
		}
		if !ok {
			return "", start, nil
		}
		return out, payloadStart + rel + 1, nil
	}
	out, ok := decodeHexSeq(payload)
	if !ok {
		return "", start, nil
	}
	return out, payloadStart + rel + 1, nil
}

// decodeHexSeq parses a hex-pair-seq — "*blank hex-pair *(1*blank hex-pair)
// *blank" where hex-pair is 1*2HEXDIG — and returns the octets it names. ok is
// false when payload does not match the grammar (a non-hex character, or a digit
// run that is not split into blank-separated 1-or-2-digit pairs), so the caller
// leaves the sequence literal.
func decodeHexSeq(payload string) (string, bool) {
	var b strings.Builder
	i, n := skipEncBlanks(payload, 0), len(payload)
	if i >= n {
		return "", false // a hex-pair-seq requires at least one hex-pair
	}
	for {
		if i >= n || !isHexDigit(payload[i]) {
			return "", false
		}
		v := hexVal(payload[i])
		i++
		if i < n && isHexDigit(payload[i]) { // optional second digit of the pair
			v = v<<4 | hexVal(payload[i])
			i++
		}
		b.WriteByte(v)
		if i >= n {
			return b.String(), true
		}
		if !isEncBlank(payload[i]) {
			return "", false // a hex-pair must be followed by a blank or the end
		}
		i = skipEncBlanks(payload, i)
		if i >= n {
			return b.String(), true // trailing blanks
		}
	}
}

// decodeUnicodeSeq parses a unicode-hex-seq — "*blank unicode-hex *(1*blank
// unicode-hex) *blank" where unicode-hex is 1*HEXDIG — and returns the UTF-8
// encoding of the code points it names. ok is false when payload does not match
// the grammar (the caller then leaves the sequence literal); err is non-nil when
// a well-formed value is a surrogate (U+D800..U+DFFF) or exceeds U+10FFFF.
func decodeUnicodeSeq(payload string) (string, bool, error) {
	var b strings.Builder
	var rangeErr error
	i, n := skipEncBlanks(payload, 0), len(payload)
	if i >= n {
		return "", false, nil
	}
	for {
		if i >= n || !isHexDigit(payload[i]) {
			return "", false, nil
		}
		p := i
		for i < n && isHexDigit(payload[i]) { // 1*HEXDIG
			i++
		}
		cp, perr := strconv.ParseUint(payload[p:i], 16, 32)
		if perr != nil || cp > 0x10FFFF || (cp >= 0xD800 && cp <= 0xDFFF) {
			// RFC 5228 §2.4.2.4 forbids a range error unless the entire
			// sequence fits the  Remember the first bad value while
			// checking the remaining separators and digit runs.
			if rangeErr == nil {
				rangeErr = fmt.Errorf("encoded-character: unicode value %q is outside the valid range (0..D7FF, E000..10FFFF)", payload[p:i])
			}
		} else {
			b.WriteRune(rune(cp))
		}
		if i >= n {
			break
		}
		if !isEncBlank(payload[i]) {
			return "", false, nil
		}
		i = skipEncBlanks(payload, i)
		if i >= n {
			break
		}
	}
	if rangeErr != nil {
		return "", true, rangeErr
	}
	return b.String(), true, nil
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// hexVal returns the value of the hex digit c, which the caller has checked
// with isHexDigit.
func hexVal(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

// isEncBlank identifies possible blank separator octets. skipEncBlanks checks
// that a line break is a complete CRLF (RFC 5228 §2.4.2.4).
func isEncBlank(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func skipEncBlanks(s string, i int) int {
	for i < len(s) {
		switch s[i] {
		case ' ', '\t':
			i++
		case '\r':
			if i+1 >= len(s) || s[i+1] != '\n' {
				return i
			}
			i += 2
		default:
			return i
		}
	}
	return i
}

// hasASCIIFoldPrefix reports whether s begins with prefix, folding only the
// US-ASCII letters A–Z (RFC 5228 §2.4.2.4 makes the "hex"/"unicode" keywords
// case-insensitive). prefix must be lowercase ASCII. It deliberately does not use
// Go's Unicode-aware case folding, so no non-ASCII octet can masquerade as a
// keyword letter.
func hasASCIIFoldPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefix[i] {
			return false
		}
	}
	return true
}
