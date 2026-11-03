package match

import (
	"regexp"
	"strings"
)

// Any reports whether any value matches any key under the selected match type
// and comparator.
func Any(values []string, matchType, comparator string, keys []string) bool {
	for _, v := range values {
		for _, k := range keys {
			if One(v, k, matchType, comparator) {
				return true
			}
		}
	}
	return false
}

// One compares one value and key using a validated match type and comparator.
func One(value, key, matchType, comparator string) bool {
	switch matchType {
	case ":contains":
		if comparator == "i;octet" {
			return strings.Contains(value, key)
		}
		return strings.Contains(ASCIIFold(value), ASCIIFold(key))
	case ":matches":
		return wildcardMatch(foldForComparator(value, comparator), foldForComparator(key, comparator))
	case ":regex":
		prefix := "(?i)"
		if comparator == "i;octet" {
			prefix = ""
		}
		re, err := regexp.Compile(prefix + key)
		if err != nil {
			return false // skip invalid regex
		}
		return re.MatchString(value)
	default:
		// ":is", the default match type (RFC 5228 §2.7.1). The parser accepts
		// no other tag, so nothing else reaches here.
		return Equal(value, key, comparator)
	}
}

// Equal implements :is for the three supported comparators.
func Equal(value, key, comparator string) bool {
	switch comparator {
	case "i;octet":
		return value == key
	case "i;ascii-numeric":
		nv, okv := asciiNumeric(value)
		nk, okk := asciiNumeric(key)
		if !okv || !okk {
			// Non-numbers are all equal to one another and unequal to numbers.
			return !okv && !okk
		}
		return nv == nk
	default: // i;ascii-casemap
		return ASCIIFold(value) == ASCIIFold(key)
	}
}

// asciiNumeric returns the value of the leading run of digits per the
// i;ascii-numeric comparator (RFC 4790 §9.1.1), as that run with its leading
// zeros removed ("0" for a run of zeros). The comparator's integers have no
// upper bound, so the value is kept as a digit string rather than parsed into a
// fixed-width integer: two runs are equal exactly when the strings are. Returns
// false when the value does not begin with a digit (positive infinity).
func asciiNumeric(s string) (string, bool) {
	// The first octet must be a digit (RFC 4790 §9.1.1). Whitespace,
	// including Unicode whitespace, denotes infinity just like any other
	// non-digit prefix. Header-specific trimming happens in headerValues.
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return "", false
	}
	digits := strings.TrimLeft(s[:end], "0")
	if digits == "" {
		return "0", true
	}
	return digits, true
}

func foldForComparator(s, comparator string) string {
	if comparator == "i;octet" {
		return s
	}
	return ASCIIFold(s)
}

// ASCIIFold applies the i;ascii-casemap comparator's case fold
// (RFC 4790 §9.2, RFC 5228 §2.7.3): ONLY the US-ASCII letters A–Z (0x41–0x5A)
// fold to a–z. Every other octet — all non-letter ASCII and every byte ≥ 0x80,
// including the interior bytes of multi-byte UTF-8 sequences — is left
// byte-exact, so non-ASCII characters are compared unchanged and cannot be
// folded onto an ASCII look-alike (e.g. ſ U+017F, Kelvin sign K U+212A). This
// deliberately avoids Go's Unicode-aware strings.ToLower/EqualFold, which fold
// beyond US-ASCII and would let a filter be evaded or over-
func ASCIIFold(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if b == nil {
				b = []byte(s)
			}
			b[i] = c + ('a' - 'A')
		}
	}
	if b == nil {
		return s
	}
	return string(b)
}

// wildcardMatch implements Sieve :matches semantics (RFC 5228 §2.7.1): '*'
// matches zero or more characters, '?' matches exactly one, and a backslash
// escapes the next character. Both value and pattern are octet strings under
// this evaluator's comparator model (i;octet, i;ascii-casemap), so a
// "character" is a single octet: '?' matches exactly one byte and '*' scans
// byte by byte. A multi-byte UTF-8 sequence is therefore NOT one '?' — it takes
// as many '?' as it has octets. The whole value must be matched (anchored).
//
// It runs a linear scan with a single backtrack point for the most recent '*',
// which is sufficient because '*' is the only construct that consumes a
// variable number of octets.
func wildcardMatch(value, pattern string) bool {
	// vi, pi index value and pattern by octet. star records the pattern index
	// just after the most recent '*'; starMatch records how much of value that
	// '*' has been credited so far, so a later mismatch can extend it by one
	// octet and retry.
	vi, pi := 0, 0
	star, starMatch := -1, 0
	for vi < len(value) {
		if pi < len(pattern) {
			switch pattern[pi] {
			case '*':
				star, starMatch = pi, vi
				pi++
				continue
			case '?':
				// '?' matches exactly one octet of value.
				vi++
				pi++
				continue
			case '\\':
				// A backslash escapes the following octet, matching it
				// literally; a trailing backslash is itself a literal '\'.
				lit := byte('\\')
				adv := 1
				if pi+1 < len(pattern) {
					lit = pattern[pi+1]
					adv = 2
				}
				if value[vi] == lit {
					vi++
					pi += adv
					continue
				}
			default:
				if value[vi] == pattern[pi] {
					vi++
					pi++
					continue
				}
			}
		}
		// Literal/'?' mismatch (or pattern exhausted): backtrack to the last
		// '*' and let it swallow one more octet, if there was one.
		if star >= 0 {
			pi = star + 1
			starMatch++
			vi = starMatch
			continue
		}
		return false
	}
	// Value fully consumed: the remaining pattern matches only if it is nothing
	// but '*' (each matching the empty string).
	for pi < len(pattern) && pattern[pi] == '*' {
		pi++
	}
	return pi == len(pattern)
}

// Captures matches value against key like [One] for the :matches and :regex
// match types and, on success, returns the match variables of RFC 5229 §3.2:
// index 0 is the matched part of value, then one string per wildcard ("*" or
// "?") of a :matches pattern, or per capture group of a :regex. At most
// limit+1 strings are returned. Wildcards match as little as possible.
func Captures(value, key, matchType, comparator string, limit int) ([]string, bool) {
	var groups []string
	if matchType == ":regex" {
		prefix := "(?i)"
		if comparator == "i;octet" {
			prefix = ""
		}
		re, err := regexp.Compile(prefix + key)
		if err != nil {
			return nil, false
		}
		loc := re.FindStringSubmatchIndex(value)
		if loc == nil {
			return nil, false
		}
		for i := 0; i+1 < len(loc); i += 2 {
			if loc[i] < 0 {
				groups = append(groups, "")
				continue
			}
			groups = append(groups, value[loc[i]:loc[i+1]])
		}
	} else {
		var ok bool
		groups, ok = wildcardCaptures(value, key, comparator)
		if !ok {
			return nil, false
		}
	}
	if len(groups) > limit+1 {
		groups = groups[:limit+1]
	}
	return groups, true
}

// wildcardCaptures is wildcardMatch that also records what each wildcard
// matched. The comparator folds only case, never length, so the folded value
// and the original share octet offsets and each capture is cut from the
// original value. Shorter matches are tried first for "*" (non-greedy), and a
// failed (value, pattern) position is remembered so the search stays
// polynomial.
func wildcardCaptures(value, pattern, comparator string) ([]string, bool) {
	fv, fp := foldForComparator(value, comparator), foldForComparator(pattern, comparator)
	failed := map[[2]int]bool{}
	var caps []string
	var try func(vi, pi int) bool
	try = func(vi, pi int) bool {
		if failed[[2]int{vi, pi}] {
			return false
		}
		n := len(caps)
		ok := func() bool {
			if pi == len(fp) {
				return vi == len(fv)
			}
			switch fp[pi] {
			case '*':
				for end := vi; end <= len(fv); end++ {
					caps = append(caps[:n], value[vi:end])
					if try(end, pi+1) {
						return true
					}
				}
				return false
			case '?':
				if vi >= len(fv) {
					return false
				}
				caps = append(caps[:n], value[vi:vi+1])
				return try(vi+1, pi+1)
			case '\\':
				lit, adv := byte('\\'), 1
				if pi+1 < len(fp) {
					lit, adv = fp[pi+1], 2
				}
				return vi < len(fv) && fv[vi] == lit && try(vi+1, pi+adv)
			default:
				return vi < len(fv) && fv[vi] == fp[pi] && try(vi+1, pi+1)
			}
		}()
		if !ok {
			caps = caps[:n]
			failed[[2]int{vi, pi}] = true
		}
		return ok
	}
	if !try(0, 0) {
		return nil, false
	}
	return append([]string{value}, caps...), true
}
