package sieve

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rest-mail/go-sieve/pkg/sieve/internal/grammar"
	"github.com/rest-mail/go-sieve/pkg/sieve/internal/match"
)

// This file implements the variables extension (RFC 5229): the set action and
// its modifiers, the string test, ${name} interpolation and match variables.
//
// Strings keep their decoded source text in the syntax tree. When a script
// declares "variables", the evaluator expands each string at the moment control
// reaches the statement that uses it (§3), in a single pass, so a value that
// itself contains "${...}" is never expanded again. Backslash quoting and
// encoded-character decoding are resolved by the parser first (§3.1).

// Implementation limits (RFC 5229 §6). Each meets or exceeds the minimum the
// RFC requires an implementation to support.
const (
	// maxVariables caps the distinct variables a script may set. The RFC
	// minimum is 128. Variable names are constant, so the parser counts them.
	maxVariables = 128
	// maxVariableNameLen caps the length of a variable name. The RFC minimum
	// is 32 characters.
	maxVariableNameLen = 400
	// maxVariableValueChars caps a value, in characters (not octets). The RFC
	// minimum is 4000 characters. A longer constant value is a parse error; a
	// longer value found at run time is truncated, never an error.
	maxVariableValueChars = 4000
	// maxMatchVariable is the highest match variable supported: ${0}..${9}.
	// A reference to a higher index is a syntax error.
	maxMatchVariable = 9
)

// Modifiers of the set action (RFC 5229 §4.1).
const (
	modLower = 1 << iota
	modUpper
	modLowerFirst
	modUpperFirst
	modQuoteWildcard
	modLength
)

// setModifiers maps each modifier tag to its bit and precedence.
var setModifiers = map[string]struct {
	bit        int
	precedence int
}{
	":lower":         {modLower, 40},
	":upper":         {modUpper, 40},
	":lowerfirst":    {modLowerFirst, 30},
	":upperfirst":    {modUpperFirst, 30},
	":quotewildcard": {modQuoteWildcard, 20},
	":length":        {modLength, 10},
}

// setCmd is the set action: it stores value, expanded and modified, in name.
type setCmd struct {
	name      string // lower case; names are case-insensitive (§4)
	value     string
	modifiers int
}

func (*setCmd) isCmd() {}

// stringTest is the string test (RFC 5229 §5): it matches script strings, after
// expansion, against keys. No whitespace is stripped.
type stringTest struct {
	comparator string
	matchType  string
	relation   string // the RFC 5231 operator of :value or :count
	sources    []string
	keys       []string
}

func (*stringTest) isTest() {}

// variableState holds the variables of one evaluation. Evaluate makes a fresh
// one for each call, so nothing is shared between evaluations (§4).
type variableState struct {
	values  map[string]string
	matches []string // most recent successful :matches; empty before one
	// requires holds the script's require names, so a comparator named at
	// run time can be checked as Parse checks a constant one.
	requires []string
}

// ── Variable references ──────────────────────────────────────────────

// isIdentifier reports whether s is an RFC 5228 identifier:
// (ALPHA / "_") *(ALPHA / DIGIT / "_").
func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '_', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

// isDigits reports whether s is a num-variable: 1*DIGIT.
func isDigits(s string) bool {
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

// variableRefAt parses the variable-ref (RFC 5229 §3) that starts at s[i]:
//
//	variable-ref  = "${" [namespace] variable-name "}"
//	namespace     = identifier "." *sub-namespace
//	sub-namespace = variable-name "."
//	variable-name = num-variable / identifier
//
// It returns the index just past the closing brace and the dot-separated
// components of the reference. ok is false when no well-formed reference
// starts at i; such text stays as it is.
func variableRefAt(s string, i int) (end int, parts []string, ok bool) {
	if !strings.HasPrefix(s[i:], "${") {
		return 0, nil, false
	}
	closing := strings.IndexByte(s[i+2:], '}')
	if closing < 0 {
		return 0, nil, false
	}
	parts = strings.Split(s[i+2:i+2+closing], ".")
	if len(parts) > 1 && !isIdentifier(parts[0]) {
		return 0, nil, false
	}
	for _, part := range parts {
		if !isIdentifier(part) && !isDigits(part) {
			return 0, nil, false
		}
	}
	return i + 2 + closing + 1, parts, true
}

// matchIndex returns the index a num-variable names. Extra leading zeroes are
// ignored (§3.2). An index too large to support reports false.
func matchIndex(digits string) (int, bool) {
	digits = strings.TrimLeft(digits, "0")
	if digits == "" {
		return 0, true
	}
	if len(digits) > 1 || int(digits[0]-'0') > maxMatchVariable {
		return 0, false
	}
	return int(digits[0] - '0'), true
}

// checkVariableRefs refuses, at parse time, the references this package cannot
// expand: a match variable above ${9} (§6 makes it a syntax error) and any
// namespace, since no supported extension defines one (§3: a reference to a
// namespace without its require MUST be an error).
func checkVariableRefs(s string) error {
	for i := 0; i < len(s); i++ {
		end, parts, ok := variableRefAt(s, i)
		if !ok {
			continue
		}
		if len(parts) > 1 {
			return fmt.Errorf("variable namespace %q is not supported", parts[0])
		}
		if isDigits(parts[0]) {
			if _, ok := matchIndex(parts[0]); !ok {
				return fmt.Errorf("match variable ${%s} is out of range: only ${0} to ${%d} are supported", parts[0], maxMatchVariable)
			}
		}
		i = end - 1
	}
	return nil
}

// hasVariableRef reports whether s holds at least one variable reference, so
// that it is not a constant string (§3).
func hasVariableRef(s string) bool {
	for i := 0; i < len(s); i++ {
		if _, _, ok := variableRefAt(s, i); ok {
			return true
		}
	}
	return false
}

// expand replaces each variable reference in s with its current value, in one
// pass (§3). Unknown variables and match variables outside the last match list
// become the empty string.
func (vs *variableState) expand(s string) string {
	if !strings.Contains(s, "${") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		end, parts, ok := variableRefAt(s, i)
		if !ok {
			b.WriteByte(s[i])
			continue
		}
		b.WriteString(vs.lookup(parts))
		i = end - 1
	}
	return b.String()
}

// lookup returns the value of a parsed reference. The parser has already
// refused namespaces and out-of-range match variables.
func (vs *variableState) lookup(parts []string) string {
	name := parts[0]
	if isDigits(name) {
		if n, ok := matchIndex(name); ok && n < len(vs.matches) {
			return vs.matches[n]
		}
		return ""
	}
	return vs.values[match.ASCIIFold(name)]
}

// setMatches records the match list of a successful :matches, keeping only the
// entries ${0}..${9} a script can reference.
func (vs *variableState) setMatches(groups []string) {
	if len(groups) > maxMatchVariable+1 {
		groups = groups[:maxMatchVariable+1]
	}
	vs.matches = groups
}

// assign stores a value, truncated to the value limit (§6: truncate at run
// time, never an error). quoted says the value went through :quotewildcard
// last, so a cut that splits an escape from the octet it quotes drops the
// dangling backslash rather than leave a different pattern.
func (vs *variableState) assign(name, value string, quoted bool) {
	if cut, truncated := truncateChars(value, maxVariableValueChars); truncated {
		value = cut
		if quoted {
			run := len(value) - len(strings.TrimRight(value, `\`))
			if run%2 == 1 {
				value = value[:len(value)-1]
			}
		}
	}
	vs.values[name] = value
}

// charCount counts characters as RFC 5229 does (§1: ISO 10646 characters in
// UTF-8). An octet that is not part of valid UTF-8 counts as one character.
func charCount(s string) int { return utf8.RuneCountInString(s) }

// truncateChars cuts s to at most limit characters, on a character boundary.
func truncateChars(s string, limit int) (string, bool) {
	count := 0
	for i := 0; i < len(s); {
		if count == limit {
			return s[:i], true
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		count++
	}
	return s, false
}

// applyModifiers applies set's modifiers in precedence order, largest first
// (§4.1): case (40), first-letter case (30), :quotewildcard (20), :length (10).
// Case changes touch only the US-ASCII letters (§4.1.3).
func applyModifiers(s string, mods int) string {
	switch {
	case mods&modLower != 0:
		s = match.ASCIIFold(s)
	case mods&modUpper != 0:
		s = asciiUpper(s)
	}
	if s != "" {
		switch c := s[0]; {
		case mods&modLowerFirst != 0 && c >= 'A' && c <= 'Z':
			s = string(c+'a'-'A') + s[1:]
		case mods&modUpperFirst != 0 && c >= 'a' && c <= 'z':
			s = string(c-('a'-'A')) + s[1:]
		}
	}
	if mods&modQuoteWildcard != 0 {
		var b strings.Builder
		b.Grow(len(s))
		for i := 0; i < len(s); i++ {
			if c := s[i]; c == '*' || c == '?' || c == '\\' {
				b.WriteByte('\\')
			}
			b.WriteByte(s[i])
		}
		s = b.String()
	}
	if mods&modLength != 0 {
		s = fmt.Sprint(charCount(s))
	}
	return s
}

// asciiUpper upper-cases only the US-ASCII letters a-z.
func asciiUpper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - ('a' - 'A')
		}
	}
	return string(b)
}

// ── Parsing ──────────────────────────────────────────────────────────

// dynamic reports whether a decoded string is not constant: the script uses
// variables and the string holds a reference. Checks that need the final value
// move from parse time to run time for such a string.
func (p *parser) dynamic(s string) bool {
	return p.required["variables"] && hasVariableRef(s)
}

// declareVariable checks a name a script assigns to (set, or a flag action's
// variable name) and counts it against maxVariables. The name must be a
// constant identifier: not a match variable and not namespaced (§4).
func (p *parser) declareVariable(tok grammar.Token) (string, error) {
	if !isIdentifier(tok.Str) {
		return "", fmt.Errorf("sieve: line %d: invalid variable name %q: it must be an identifier (match variables and namespaces cannot be set)", tok.Line, tok.Str)
	}
	if len(tok.Str) > maxVariableNameLen {
		return "", fmt.Errorf("sieve: line %d: variable name is longer than %d characters", tok.Line, maxVariableNameLen)
	}
	name := match.ASCIIFold(tok.Str)
	if !p.variableNames[name] {
		if len(p.variableNames) == maxVariables {
			return "", fmt.Errorf("sieve: line %d: more than %d distinct variables", tok.Line, maxVariables)
		}
		p.variableNames[name] = true
	}
	return name, nil
}

// parseSet parses: set [MODIFIER...] <name: string> <value: string> (§4).
func (p *parser) parseSet() (sieveCmd, error) {
	cmd := &setCmd{}
	used := map[int]string{}
	for p.peek().Kind == grammar.Tag {
		tag := p.next().Str // the lexer lower-cases tags: modifiers are case-insensitive
		mod, ok := setModifiers[tag]
		if !ok {
			return nil, p.errf("unknown set modifier %q", tag)
		}
		if prev, clash := used[mod.precedence]; clash {
			return nil, p.errf("set modifiers %s and %s have the same precedence and cannot be combined", prev, tag)
		}
		used[mod.precedence] = tag
		cmd.modifiers |= mod.bit
	}
	nameTok, err := p.expect(grammar.String, "a variable name")
	if err != nil {
		return nil, err
	}
	if cmd.name, err = p.declareVariable(nameTok); err != nil {
		return nil, err
	}
	value, err := p.expect(grammar.String, "a value")
	if err != nil {
		return nil, err
	}
	cmd.value = value.Str
	// §6: a value over the limit SHOULD be a compile-time error when it can be
	// known then, which it can for a constant value.
	if !hasVariableRef(cmd.value) && charCount(applyModifiers(cmd.value, cmd.modifiers)) > maxVariableValueChars {
		return nil, fmt.Errorf("sieve: line %d: set value is longer than %d characters", value.Line, maxVariableValueChars)
	}
	return cmd, p.finishStatement()
}

// parseStringTest parses: string [MATCH-TYPE] [COMPARATOR] <source> <keys> (§5).
// With "relational", MATCH-TYPE may be :value or :count (RFC 5231 §4).
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
	return &stringTest{comparator: opts.comparator, matchType: opts.matchType, relation: opts.relation, sources: sources, keys: keys}, nil
}

// ── Evaluation ───────────────────────────────────────────────────────

// expand returns s with its variables expanded, or s unchanged when the script
// does not use variables.
func (st *evalState) expand(s string) string {
	if st.vars == nil {
		return s
	}
	return st.vars.expand(s)
}

// expandList expands every string of a list.
func (st *evalState) expandList(list []string) []string {
	if st.vars == nil {
		return list
	}
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = st.vars.expand(s)
	}
	return out
}

// matchValues compares values with the expanded keys. With variables, a
// successful :matches sets the match variables from the first pair that
// matches (§3.2); a failed one leaves them as they were.
func (st *evalState) matchValues(values []string, matchType, relation, comparator string, keys []string) bool {
	comparator = st.expandComparator(matchType, comparator)
	return st.matchExpanded(values, matchType, st.expandRelation(matchType, relation), comparator, st.expandList(keys))
}

// expandComparator returns a test's comparator name. Parse has checked a
// constant one; one built from variables (RFC 5229 §3) is expanded here and
// checked the same way: case-insensitive, built in or declared with require
// (RFC 5228 §2.7.3), and able to serve the match type (RFC 4790 §9.1). A bad
// value is a runtime error, so the fail-safe keep applies.
func (st *evalState) expandComparator(matchType, comparator string) string {
	if st.vars == nil || !hasVariableRef(comparator) {
		return comparator
	}
	expanded := st.vars.expand(comparator)
	name := match.ASCIIFold(expanded)
	if name != defaultComparator && name != "i;octet" && !slices.Contains(st.vars.requires, "comparator-"+name) {
		panic(fmt.Errorf("sieve: comparator %q is neither built in nor declared with require (RFC 5228 §2.7.3)", expanded))
	}
	if err := checkComparatorMatchType(name, matchType); err != nil {
		panic(fmt.Errorf("sieve: %w", err))
	}
	return name
}

// expandRelation returns the relational operator of a :value or :count test.
// With variables it is expanded first (RFC 5229 §3), and an expansion that is
// not an operator is a runtime error, so the fail-safe keep applies. Any other
// match type has no operator.
func (st *evalState) expandRelation(matchType, relation string) string {
	if st.vars == nil || (matchType != ":value" && matchType != ":count") {
		return relation
	}
	expanded := st.vars.expand(relation)
	op, ok := match.Relation(expanded)
	if !ok {
		panic(fmt.Errorf("sieve: unknown relational operator %q after %s: %s", expanded, matchType, relationalOperators))
	}
	return op
}

// matchExpanded is matchValues for keys already expanded. Only :matches sets
// match variables; the relational :value and :count set none.
func (st *evalState) matchExpanded(values []string, matchType, relation, comparator string, keys []string) bool {
	if st.vars == nil || matchType != ":matches" {
		return matchPlain(values, matchType, relation, comparator, keys)
	}
	for _, v := range values {
		for _, k := range keys {
			if groups, ok := match.Captures(v, k, comparator); ok {
				st.vars.setMatches(groups)
				return true
			}
		}
	}
	return false
}

// runSet executes the set action.
func (st *evalState) runSet(cmd *setCmd) {
	value := applyModifiers(st.vars.expand(cmd.value), cmd.modifiers)
	quoted := cmd.modifiers&modQuoteWildcard != 0 && cmd.modifiers&modLength == 0
	st.vars.assign(cmd.name, value, quoted)
}

// runFlagVariable applies setflag/addflag/removeflag to a named variable rather
// than the internal flag variable (RFC 5232 §3). The variable holds a
// space-separated flag list, cleaned up like any other list of flags.
func (st *evalState) runFlagVariable(op, name string, flags []string) {
	current := normalizeFlags([]string{st.vars.values[name]})
	var result []string
	switch op {
	case "setflag":
		result = flags
	case "addflag":
		result = normalizeFlags(append(current, flags...))
	default: // removeflag
		remove := map[string]bool{}
		for _, f := range flags {
			remove[strings.ToLower(f)] = true
		}
		for _, f := range current {
			if !remove[strings.ToLower(f)] {
				result = append(result, f)
			}
		}
	}
	st.vars.assign(name, strings.Join(result, " "), false)
}
