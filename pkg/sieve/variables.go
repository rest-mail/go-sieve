package sieve

import (
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/rest-mail/go-sieve/pkg/sieve/internal/grammar"
	"github.com/rest-mail/go-sieve/pkg/sieve/internal/match"
)

// This file implements the variables extension (RFC 5229): the set action and
// its modifiers, ${name} interpolation in string arguments, the match
// variables ${0}..${9} and the string test.
//
// Strings are kept as written (after quoting and encoded-character decoding,
// RFC 5229 §3.1) and expanded when control reaches the test or action they are
// part of (§3). That includes a comparator name and the relational operator
// of :value and :count: Parse checks a constant one, and one built from
// variables is checked once expanded, when its test runs.

// Implementation limits (RFC 5229 §6).
const (
	// MaxVariables is the number of distinct variables a script may set. RFC
	// 5229 §6 requires at least 128; set names are constants, so a script
	// that names more is refused by Parse.
	MaxVariables = 256
	// MaxVariableValue is the longest value, in octets, a variable holds. RFC
	// 5229 §6 requires at least 4000 characters, which is up to 16000 octets
	// of UTF-8. A constant value that is longer is refused by Parse; a longer
	// value built at run time is truncated at a character boundary (§6: it
	// MUST NOT be treated as an error).
	MaxVariableValue = 16384
	// maxMatchVariable is the highest match variable index supported: ${0}
	// to ${9} (RFC 5229 §6). A reference to a higher index is a syntax error.
	maxMatchVariable = 9
)

// setCmd is the RFC 5229 §4 set action. name is the lower-cased variable
// name; modifiers are already ordered by precedence, largest first.
type setCmd struct {
	name      string
	modifiers []string
	value     string
}

func (*setCmd) isCmd() {}

// stringTest is the RFC 5229 §5 string test.
type stringTest struct {
	comparator string
	matchType  string
	relation   string
	sources    []string
	keys       []string
}

func (*stringTest) isTest() {}

// modifierPrecedence is the RFC 5229 §4.1 precedence of each set modifier.
var modifierPrecedence = map[string]int{
	":lower":         40,
	":upper":         40,
	":lowerfirst":    30,
	":upperfirst":    30,
	":quotewildcard": 20,
	":length":        10,
}

// variableRef is one variable-ref found in a string (RFC 5229 §3).
type variableRef struct {
	start, end int    // the ref is s[start:end], "${" through "}"
	namespace  string // empty, or every component before the last, joined by "."
	name       string // the variable-name, as written
}

// nextVariableRef finds the first variable-ref in s at or after from. A "${"
// that does not begin a well-formed reference is ordinary text, so the scan
// continues one octet after it: "${BAD${Company}" holds the reference
// "${Company}" (§3).
func nextVariableRef(s string, from int) (variableRef, bool) {
	for {
		i := strings.Index(s[from:], "${")
		if i < 0 {
			return variableRef{}, false
		}
		start := from + i
		j := start + 2
		for j < len(s) && (isIdentChar(s[j]) || s[j] == '.') {
			j++
		}
		if j < len(s) && s[j] == '}' {
			if ns, name, ok := splitVariableRef(s[start+2 : j]); ok {
				return variableRef{start: start, end: j + 1, namespace: ns, name: name}, true
			}
		}
		from = start + 1
	}
}

// splitVariableRef parses the inside of "${...}" by the RFC 5229 §3 grammar:
// [namespace] variable-name, where namespace = identifier "." *(variable-name
// "."), and variable-name = num-variable / identifier.
func splitVariableRef(body string) (namespace, name string, ok bool) {
	parts := strings.Split(body, ".")
	for i, part := range parts {
		switch {
		case i == 0 && len(parts) > 1:
			if !isIdentifier(part) {
				return "", "", false
			}
		case !isIdentifier(part) && !isNumVariable(part):
			return "", "", false
		}
	}
	last := len(parts) - 1
	return strings.Join(parts[:last], "."), parts[last], true
}

func isIdentChar(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

// isIdentifier reports whether s is an RFC 5228 §8.1 identifier:
// (ALPHA / "_") *(ALPHA / DIGIT / "_").
func isIdentifier(s string) bool {
	if s == "" || s[0] >= '0' && s[0] <= '9' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isIdentChar(s[i]) {
			return false
		}
	}
	return true
}

// isNumVariable reports whether s is a num-variable, 1*DIGIT.
func isNumVariable(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// matchIndex returns the index a match variable name selects. Extra leading
// zeros are ignored (§3.2); ok is false above the supported ${9}.
func matchIndex(name string) (int, bool) {
	digits := strings.TrimLeft(name, "0")
	if digits == "" {
		return 0, true
	}
	if len(digits) > 1 {
		return 0, false
	}
	n, _ := strconv.Atoi(digits)
	return n, n <= maxMatchVariable
}

// hasVariableRef reports whether s is not a constant string (§3).
func hasVariableRef(s string) bool {
	_, ok := nextVariableRef(s, 0)
	return ok
}

// checkVariableRefs refuses the references RFC 5229 makes errors: a namespace
// no required extension defines (§3; no extension this package implements
// defines one), and a match variable above ${9} (§6).
func checkVariableRefs(s string) error {
	for from := 0; ; {
		ref, ok := nextVariableRef(s, from)
		if !ok {
			return nil
		}
		from = ref.end
		if ref.namespace != "" {
			return fmt.Errorf("variable namespace %q is not defined by any required extension (RFC 5229 §3)", ref.namespace)
		}
		if isNumVariable(ref.name) {
			if _, ok := matchIndex(ref.name); !ok {
				return fmt.Errorf("match variable ${%s} is out of range: only ${0} to ${%d} are supported (RFC 5229 §6)", ref.name, maxMatchVariable)
			}
		}
	}
}

// checkVariableName applies the RFC 5229 §4 rules for a name that is set: a
// constant identifier, not a match variable and without a namespace.
func checkVariableName(name string) error {
	if !isIdentifier(name) {
		if isNumVariable(name) {
			return fmt.Errorf("match variable %q cannot be set (RFC 5229 §4)", name)
		}
		return fmt.Errorf("invalid variable name %q: a name is an identifier (RFC 5229 §4)", name)
	}
	return nil
}

// declareVariable records a name the script sets and enforces MaxVariables.
func (p *parser) declareVariable(tok grammar.Token) (string, error) {
	if err := checkVariableName(tok.Str); err != nil {
		return "", fmt.Errorf("sieve: line %d: %s", tok.Line, err)
	}
	name := match.ASCIIFold(tok.Str)
	if p.variables == nil {
		p.variables = map[string]bool{}
	}
	if !p.variables[name] && len(p.variables) >= MaxVariables {
		return "", fmt.Errorf("sieve: line %d: too many variables: at most %d distinct variables are supported (RFC 5229 §6)", tok.Line, MaxVariables)
	}
	p.variables[name] = true
	return name, nil
}

// parseSet parses set [MODIFIER] <name: string> <value: string> (§4).
func (p *parser) parseSet() (sieveCmd, error) {
	cmd := &setCmd{}
	byPrecedence := map[int]string{}
	for p.peek().Kind == grammar.Tag {
		tag := p.next().Str
		prec, ok := modifierPrecedence[tag]
		if !ok {
			return nil, p.errf("unknown set modifier %q (RFC 5229 §4.1)", tag)
		}
		if other, dup := byPrecedence[prec]; dup {
			return nil, p.errf("set modifiers %s and %s have the same precedence and cannot be combined (RFC 5229 §4.1)", other, tag)
		}
		byPrecedence[prec] = tag
	}
	for _, prec := range []int{40, 30, 20, 10} {
		if tag, ok := byPrecedence[prec]; ok {
			cmd.modifiers = append(cmd.modifiers, tag)
		}
	}
	name, err := p.expect(grammar.String, "a variable name")
	if err != nil {
		return nil, err
	}
	if cmd.name, err = p.declareVariable(name); err != nil {
		return nil, err
	}
	value, err := p.expect(grammar.String, "a value")
	if err != nil {
		return nil, err
	}
	if !hasVariableRef(value.Str) && len(applyModifiers(value.Str, cmd.modifiers)) > MaxVariableValue {
		return nil, fmt.Errorf("sieve: line %d: set value is longer than the %d octets a variable holds (RFC 5229 §6)", value.Line, MaxVariableValue)
	}
	cmd.value = value.Str
	return cmd, p.finishStatement()
}

// parseStringTest parses string [MATCH-TYPE] [COMPARATOR] <source> <keys> (§5).
func (p *parser) parseStringTest() (sieveTest, error) {
	opts, err := p.parseMatchOptions(false, false)
	if err != nil {
		return nil, err
	}
	sources, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	keys, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	return &stringTest{
		comparator: opts.comparator,
		matchType:  opts.matchType,
		relation:   opts.relation,
		sources:    sources,
		keys:       keys,
	}, nil
}

// applyModifiers applies the set modifiers, already in precedence order.
func applyModifiers(v string, modifiers []string) string {
	for _, m := range modifiers {
		switch m {
		case ":lower":
			v = match.ASCIIFold(v)
		case ":upper":
			v = asciiUpper(v)
		case ":lowerfirst":
			if v != "" && v[0] >= 'A' && v[0] <= 'Z' {
				v = string(v[0]+'a'-'A') + v[1:]
			}
		case ":upperfirst":
			if v != "" && v[0] >= 'a' && v[0] <= 'z' {
				v = string(v[0]-('a'-'A')) + v[1:]
			}
		case ":quotewildcard":
			var b strings.Builder
			for i := 0; i < len(v); i++ {
				if c := v[i]; c == '*' || c == '?' || c == '\\' {
					b.WriteByte('\\')
				}
				b.WriteByte(v[i])
			}
			v = b.String()
		case ":length":
			v = strconv.Itoa(utf8.RuneCountInString(v))
		}
	}
	return v
}

// asciiUpper converts only the US-ASCII letters a-z (§4.1.3).
func asciiUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}

// truncateValue cuts v to MaxVariableValue octets without splitting a UTF-8
// character (§6).
func truncateValue(v string) string {
	if len(v) <= MaxVariableValue {
		return v
	}
	cut := MaxVariableValue
	for cut > 0 && !utf8.RuneStart(v[cut]) {
		cut--
	}
	return v[:cut]
}

// varState is the run-time variable state of one evaluation.
type varState struct {
	vars map[string]string
	// matched holds the match variables of the most recent successful
	// :matches (or :regex) match; index 0 is the matched source value.
	matched []string
	// declared holds the script's require names, for a comparator named
	// at run time.
	declared map[string]bool
}

// expand interpolates every variable-ref in s, in one pass (§3). Without the
// variables extension, strings are returned unchanged.
func (st *evalState) expand(s string) string {
	if st.vs == nil || !strings.Contains(s, "${") {
		return s
	}
	var b strings.Builder
	from := 0
	for {
		ref, ok := nextVariableRef(s, from)
		if !ok {
			b.WriteString(s[from:])
			return b.String()
		}
		b.WriteString(s[from:ref.start])
		b.WriteString(st.lookup(ref.name))
		from = ref.end
	}
}

// lookup returns a variable's value; an unknown variable, or a match variable
// beyond the list, is the empty string (§3, §3.2).
func (st *evalState) lookup(name string) string {
	if isNumVariable(name) {
		if i, ok := matchIndex(name); ok && i < len(st.vs.matched) {
			return st.vs.matched[i]
		}
		return ""
	}
	return st.vs.vars[match.ASCIIFold(name)]
}

func (st *evalState) expandList(list []string) []string {
	if st.vs == nil {
		return list
	}
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = st.expand(s)
	}
	return out
}

func (st *evalState) runSet(cmd *setCmd) {
	st.vs.vars[cmd.name] = truncateValue(applyModifiers(st.expand(cmd.value), cmd.modifiers))
}

// runFlagVariable applies setflag/addflag/removeflag to a variable holding a
// list of flags instead of the internal variable (RFC 5232 §3).
func (st *evalState) runFlagVariable(cmd *flagCmd, flags []string) {
	current := normalizeFlags([]string{st.vs.vars[cmd.variable]})
	switch cmd.op {
	case "setflag":
		current = flags
	case "addflag":
		current = normalizeFlags(append(current, flags...))
	case "removeflag":
		drop := map[string]bool{}
		for _, f := range flags {
			drop[strings.ToLower(f)] = true
		}
		kept := current[:0]
		for _, f := range current {
			if !drop[strings.ToLower(f)] {
				kept = append(kept, f)
			}
		}
		current = kept
	}
	st.vs.vars[cmd.variable] = truncateValue(strings.Join(current, " "))
}

// matchValues applies a test's match type to the values, with the keys
// expanded first. When the variables extension is in use, a successful
// :matches or :regex records the match variables (§3.2); a failed match
// leaves them unchanged. setsMatch is false for the body test, whose
// wildcards have no side effects (RFC 5173 §6).
func (st *evalState) matchValues(values []string, matchType, relation, comparator string, keys []string, setsMatch bool) bool {
	keys = st.expandList(keys)
	if matchType == ":value" {
		return match.AnyValue(values, relation, comparator, keys)
	}
	if st.vs == nil || !setsMatch || (matchType != ":matches" && matchType != ":regex") {
		return match.Any(values, matchType, comparator, keys)
	}
	for _, v := range values {
		for _, k := range keys {
			if groups, ok := match.Captures(v, k, matchType, comparator, maxMatchVariable); ok {
				st.vs.matched = groups
				return true
			}
		}
	}
	return false
}

// matchArgs returns a test's comparator and relational operator, expanding
// either one that holds a variable-ref (RFC 5229 §3) and checking it as Parse
// checks a constant: the comparator must be built in or declared with require
// (RFC 5228 §2.7.3), must support the match type, and the operator must be
// one RFC 5231 §4 defines. A bad value is a run-time error: the script stops
// and the message is kept (RFC 5228 §2.10.6).
func (st *evalState) matchArgs(comparator, matchType, relation string) (string, string) {
	if st.vs == nil {
		return comparator, relation
	}
	if hasVariableRef(comparator) {
		comparator = match.ASCIIFold(st.expand(comparator))
		if comparator != defaultComparator && comparator != "i;octet" && !st.vs.declared["comparator-"+comparator] {
			panic(fmt.Errorf("sieve: comparator %q is neither built in nor declared with require (RFC 5228 §2.7.3)", comparator))
		}
		if err := checkComparatorMatchType(comparator, matchType); err != nil {
			panic(fmt.Errorf("sieve: %w", err))
		}
	}
	if hasVariableRef(relation) {
		op, ok := match.Relation(st.expand(relation))
		if !ok {
			panic(fmt.Errorf("sieve: unknown relational operator %q after %s: expected one of \"gt\", \"ge\", \"lt\", \"le\", \"eq\", \"ne\" (RFC 5231 §4)", st.expand(relation), matchType))
		}
		relation = op
	}
	return comparator, relation
}

// stringCount is the :count of the string test: each non-empty source counts
// one (RFC 5229 §5).
func stringCount(sources []string) int {
	n := 0
	for _, s := range sources {
		if s != "" {
			n++
		}
	}
	return n
}

// expandVacation returns cmd with its string arguments expanded. A :from
// built from variables is checked once expanded, as Parse checks a constant
// one (RFC 5230 §4.4).
func (st *evalState) expandVacation(cmd *vacationCmd) *vacationCmd {
	if st.vs == nil {
		return cmd
	}
	out := *cmd
	out.subject = st.expand(cmd.subject)
	out.body = st.expand(cmd.body)
	out.handle = st.expand(cmd.handle)
	out.addresses = st.expandList(cmd.addresses)
	if hasVariableRef(cmd.from) {
		out.from = st.expand(cmd.from)
		if _, err := mail.ParseAddressList(out.from); err != nil {
			panic(fmt.Errorf("sieve: invalid vacation :from mailbox-list: %w", err))
		}
	}
	return &out
}
