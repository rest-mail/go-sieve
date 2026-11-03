package match

import "strings"

// Relations lists the relational operators of RFC 5231 §4, in the RFC's order.
var Relations = []string{"gt", "ge", "lt", "le", "eq", "ne"}

// Relation returns the operator named by s, which RFC 5231 §4 spells as an
// ABNF literal and so matches without regard to ASCII case (RFC 5234 §2.3).
// ok is false for any other string.
func Relation(s string) (op string, ok bool) {
	op = ASCIIFold(s)
	for _, r := range Relations {
		if op == r {
			return op, true
		}
	}
	return "", false
}

// Compare orders value against key under comparator, returning -1, 0 or +1
// (RFC 4790 §4.3 "ordering"):
//
//   - i;octet compares the octets as unsigned bytes, a proper prefix sorting
//     first (RFC 4790 §9.3).
//   - i;ascii-casemap folds A-Z to a-z and then orders as i;octet (§9.2).
//   - i;ascii-numeric compares the leading digit runs as unbounded unsigned
//     integers; a string that does not start with a digit is positive
//     infinity, and all such strings are equal to one another (§9.1.1).
func Compare(value, key, comparator string) int {
	switch comparator {
	case "i;octet":
		return strings.Compare(value, key)
	case "i;ascii-numeric":
		nv, okv := asciiNumeric(value)
		nk, okk := asciiNumeric(key)
		switch {
		case !okv && !okk:
			return 0
		case !okv:
			return 1
		case !okk:
			return -1
		}
		// Both are digit runs with leading zeros removed, so a longer run is
		// the larger number and equal lengths order digit by digit.
		if len(nv) != len(nk) {
			if len(nv) < len(nk) {
				return -1
			}
			return 1
		}
		return strings.Compare(nv, nk)
	default: // i;ascii-casemap
		return strings.Compare(ASCIIFold(value), ASCIIFold(key))
	}
}

// Holds reports whether the relation op holds for an ordering result c from
// [Compare], where the message value is the left side (RFC 5231 §4.1).
func Holds(c int, op string) bool {
	switch op {
	case "gt":
		return c > 0
	case "ge":
		return c >= 0
	case "lt":
		return c < 0
	case "le":
		return c <= 0
	case "eq":
		return c == 0
	default: // "ne"; the parser accepts no other operator.
		return c != 0
	}
}

// AnyValue implements the :value match type (RFC 5231 §4.1): it reports
// whether any pair of message value (left side) and key (right side) satisfies
// op under comparator.
func AnyValue(values []string, op, comparator string, keys []string) bool {
	for _, v := range values {
		for _, k := range keys {
			if Holds(Compare(v, k, comparator), op) {
				return true
			}
		}
	}
	return false
}
