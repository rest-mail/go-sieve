package sieve

import (
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/rest-mail/go-sieve/pkg/sieve/internal/grammar"
	"github.com/rest-mail/go-sieve/pkg/sieve/internal/match"
)

// This file implements a recursive-descent parser over internal grammar tokens for a
// practical subset of the Sieve mail-filtering language (RFC 5228), plus a
// few widely used extensions (envelope, body, imap4flags, vacation, notify)
// and a non-standard :regex match type retained for backwards compatibility.
//
// The parser produces an AST (Script -> commands -> tests) that the evaluator
// in eval.go walks at execution time. Parsing enforces RFC 5228 "require"
// semantics: every extension a script uses must be declared with require, a
// require of an extension this package does not implement is an error, require
// must precede every other command, and an unknown command or test (including a
// typo) is an error rather than a silent no-op. See supportedCapabilities for
// the set of extensions this package implements.

// ── AST ──────────────────────────────────────────────────────────────

// Script is a fully parsed Sieve script. Parse it with [Parse]; evaluate it
// with [Script.Evaluate].
type Script struct {
	requires []string
	commands []sieveCmd
}

// Requires returns the extensions the script declared via "require".
func (s *Script) Requires() []string { return s.requires }

// Empty reports whether the script has no executable commands (a nil script, or
// one consisting only of require/comments).
func (s *Script) Empty() bool { return s == nil || len(s.commands) == 0 }

// sieveCmd is a single command (action or control structure).
type sieveCmd interface{ isCmd() }

// ifCmd represents an if / elsif* / else? chain. Branches are evaluated in
// order; the first whose test passes (or the trailing else, whose test is nil)
// has its block executed.
type ifCmd struct {
	branches []sieveBranch
}

type sieveBranch struct {
	test  sieveTest // nil for the trailing "else"
	block []sieveCmd
}

type stopCmd struct{}
type keepCmd struct{}
type discardCmd struct{}

type fileintoCmd struct {
	folder string
	create bool
	copy   bool // RFC 3894 :copy — targets a destination distinct from a consuming fileinto
}

type redirectCmd struct {
	addr string
	copy bool // RFC 3894 :copy — targets a destination distinct from a consuming redirect
}
type rejectCmd struct {
	reason   string
	extended bool
}

// flagCmd covers setflag / addflag / removeflag from the imap4flags extension.
type flagCmd struct {
	op    string // "setflag", "addflag", "removeflag"
	flags []string
}

type vacationCmd struct {
	// interval is the minimum period between auto-replies to the same sender.
	// It is carried as a duration so a :seconds argument (RFC 6131) keeps its
	// sub-day precision instead of being truncated to whole days.
	interval   time.Duration
	subject    string
	body       string
	from       string
	addresses  []string
	mime       bool
	handle     string
	handleSet  bool
	subjectSet bool
}

type notifyCmd struct {
	method  string
	message string
}

func (*ifCmd) isCmd()       {}
func (*stopCmd) isCmd()     {}
func (*keepCmd) isCmd()     {}
func (*discardCmd) isCmd()  {}
func (*fileintoCmd) isCmd() {}
func (*redirectCmd) isCmd() {}
func (*rejectCmd) isCmd()   {}
func (*flagCmd) isCmd()     {}
func (*vacationCmd) isCmd() {}
func (*notifyCmd) isCmd()   {}

// sieveTest is a boolean test used inside a control command.
type sieveTest interface{ isTest() }

type allofTest struct{ tests []sieveTest }
type anyofTest struct{ tests []sieveTest }
type notTest struct{ inner sieveTest }
type boolTest struct{ val bool }
type existsTest struct{ headers []string }

type sizeTest struct {
	over  bool
	limit int64
}

type headerTest struct {
	comparator string
	matchType  string
	headers    []string
	keys       []string
}

type addressTest struct {
	comparator  string
	matchType   string
	addressPart string
	headers     []string
	keys        []string
}

type envelopeTest struct {
	comparator  string
	matchType   string
	addressPart string
	parts       []string
	keys        []string
}

type bodyTest struct {
	comparator string
	matchType  string
	// transform is the body transform (RFC 5173 §5): ":text" (the default),
	// ":raw" or ":content". contentTypes holds the :content list.
	transform    string
	contentTypes []string
	keys         []string
}

func (*allofTest) isTest()    {}
func (*anyofTest) isTest()    {}
func (*notTest) isTest()      {}
func (*boolTest) isTest()     {}
func (*existsTest) isTest()   {}
func (*sizeTest) isTest()     {}
func (*headerTest) isTest()   {}
func (*addressTest) isTest()  {}
func (*envelopeTest) isTest() {}
func (*bodyTest) isTest()     {}

// Defaults per RFC 5228, and RFC 5173 §5 for the body transform.
const (
	defaultMatchType     = ":is"
	defaultComparator    = "i;ascii-casemap"
	defaultAddressPart   = ":all"
	defaultBodyTransform = ":text"
)

// supportedCapabilities is the set of extension capability strings this package
// implements. RFC 5228 §2.10.5 requires that a script which "require"s a
// capability outside this set be rejected rather than run, and that any
// extension feature (command, test, match type, comparator, or tagged argument)
// be declared with require before it is used. Core RFC 5228 constructs — the
// control commands, keep/discard/redirect/stop, the address/header/exists/size/
// allof/anyof/not/true/false tests, the :is/:contains/:matches match types, the
// i;ascii-casemap and i;octet comparators, and the :all/:localpart/:domain
// address parts — need no require and are not listed here.
var supportedCapabilities = map[string]bool{
	"fileinto":                   true, // RFC 5228 fileinto action
	"reject":                     true, // RFC 5429 reject action
	"ereject":                    true, // RFC 5429 ereject action
	"envelope":                   true, // RFC 5228 envelope test
	"body":                       true, // RFC 5173 body test
	"imap4flags":                 true, // RFC 5232 setflag/addflag/removeflag and :flags
	"vacation":                   true, // RFC 5230 vacation action
	"vacation-seconds":           true, // RFC 6131 :seconds, implies vacation
	"notify":                     true, // notification action
	"mailbox":                    true, // RFC 5490 fileinto :create
	"copy":                       true, // RFC 3894 :copy tagged argument
	"encoded-character":          true, // RFC 5228 §2.4.2.4 ${hex:..}/${unicode:..} in strings
	"regex":                      true, // non-standard :regex match type
	"comparator-i;ascii-numeric": true, // RFC 4790/5228 i;ascii-numeric comparator
	"comparator-i;octet":         true, // built-in comparator (may be declared explicitly)
	"comparator-i;ascii-casemap": true, // built-in comparator (may be declared explicitly)
}

// DefaultMaxDepth is the default limit on how deeply tests and control blocks
// may nest before [Parse] rejects a script. The parser and evaluator both
// recurse over this nesting, so an unbounded script would otherwise exhaust the
// goroutine stack and crash the host process (a denial-of-service vector when
// script content is user-supplied). RFC 5228 §2.10.7 explicitly sanctions a
// finite limit and requires implementations to support only 15 levels; 64
// leaves generous headroom for real scripts while capping the recursion a
// crafted script can drive. Override it per-call with [WithMaxDepth].
const DefaultMaxDepth = 64

// Option configures optional [Parse] / [Validate] behaviour.
type Option func(*parseOptions)

type parseOptions struct {
	maxDepth int
}

func defaultParseOptions() parseOptions {
	return parseOptions{maxDepth: DefaultMaxDepth}
}

// WithMaxDepth overrides [DefaultMaxDepth], the maximum nesting depth of tests
// (not / allof / anyof) and control blocks (if / elsif / else) the parser will
// accept before returning an error. A non-positive value restores the default.
func WithMaxDepth(n int) Option {
	return func(o *parseOptions) {
		if n <= 0 {
			n = DefaultMaxDepth
		}
		o.maxDepth = n
	}
}

// ── Parser ───────────────────────────────────────────────────────────

type parser struct {
	toks     []grammar.Token
	pos      int
	requires []string

	required       map[string]bool // capabilities declared via require
	seenNonRequire bool            // a non-require command has been parsed

	maxDepth   int // nesting cap for tests and blocks
	testDepth  int // current test-expression nesting
	blockDepth int // current control-block nesting
}

// Parse parses a Sieve script into an evaluable [Script]. It is strict about
// the constructs it understands and lenient about unknown extensions (see the
// package overview). Raw NUL is forbidden throughout the source, including
// comments. Bare CR is forbidden throughout the source; encoded-character
// may produce NUL and bare CR in string values.
// Nesting of tests and control blocks is capped at
// [DefaultMaxDepth] (override with [WithMaxDepth]); a script that exceeds the
// cap is rejected rather than allowed to exhaust the stack at parse or
// evaluation time. Reject/ereject and vacation on the same control path are
// incompatible (RFC 5429 §2.4); independent conditions are conservatively
// assumed able to match together. Use alternatives of one if/elsif/else chain
// when the actions are mutually exclusive.
func Parse(src string, opts ...Option) (*Script, error) { return parseSieveScript(src, opts...) }

func parseSieveScript(src string, opts ...Option) (*Script, error) {
	cfg := defaultParseOptions()
	for _, opt := range opts {
		opt(&cfg)
	}
	toks, err := grammar.New(src).Tokenize()
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, maxDepth: cfg.maxDepth, required: map[string]bool{}}
	// At top level parseCommands returns without an error only at EOF; a
	// stray "}" or any other leftover token is already an error there.
	cmds, err := p.parseCommands(false)
	if err != nil {
		return nil, err
	}
	if _, err := checkActionCompatibility(cmds, actionUsage{}); err != nil {
		return nil, err
	}
	return &Script{requires: p.requires, commands: cmds}, nil
}

// actionUsage records actions that can precede the next command. All action
// names are static in this language, so incompatibilities can be refused before
// Evaluate hands any action to the host. Independent conditions are treated
// conservatively; alternatives within a single if/elsif/else are exclusive.
// Even actions after stop/reject are checked, so reversing their source order
// cannot hide an invalid combination behind a terminal command.
type actionUsage struct {
	reject   bool
	vacation bool
}

func checkActionCompatibility(cmds []sieveCmd, seen actionUsage) (actionUsage, error) {
	for _, c := range cmds {
		switch cmd := c.(type) {
		case *rejectCmd:
			if seen.vacation {
				return seen, fmt.Errorf("sieve: reject/ereject and vacation are incompatible")
			}
			seen.reject = true
		case *vacationCmd:
			if seen.reject {
				return seen, fmt.Errorf("sieve: reject/ereject and vacation are incompatible")
			}
			seen.vacation = true
		case *ifCmd:
			before := seen
			for _, branch := range cmd.branches {
				usage, err := checkActionCompatibility(branch.block, before)
				if err != nil {
					return seen, err
				}
				seen.reject = seen.reject || usage.reject
				seen.vacation = seen.vacation || usage.vacation
			}
		}
	}
	return seen, nil
}

func (p *parser) peek() grammar.Token { return p.toks[p.pos] }

func (p *parser) next() grammar.Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) errf(format string, args ...interface{}) error {
	return fmt.Errorf("sieve: line %d: %s", p.peek().Line, fmt.Sprintf(format, args...))
}

func (p *parser) describe(t grammar.Token) string {
	switch t.Kind {
	case grammar.EOF:
		return "end of script"
	case grammar.Ident:
		return fmt.Sprintf("identifier %q", t.Str)
	case grammar.Tag:
		return fmt.Sprintf("tag %q", t.Str)
	case grammar.String:
		return "string"
	case grammar.Number:
		return "number"
	default:
		return "token"
	}
}

func (p *parser) expect(k grammar.Kind, what string) (grammar.Token, error) {
	if p.peek().Kind != k {
		return grammar.Token{}, p.errf("expected %s, got %s", what, p.describe(p.peek()))
	}
	tok := p.next()
	if k == grammar.String {
		s, err := p.decodeStringConstant(tok)
		if err != nil {
			return grammar.Token{}, err
		}
		tok.Str = s
	}
	return tok, nil
}

// decodeStringConstant applies the RFC 5228 §2.4.2.4 "encoded-character"
// transformation to a string token's value when — and only when — the script
// declared require "encoded-character". Capability names in require itself are
// consumed before the capability is recorded, so they are never decoded. Every
// other string constant becomes its decoded value at parse time; the AST and the
// evaluator therefore see the intended octets. A well-formed but out-of-range
// ${unicode:...} value is reported as an error against the token's line.
func (p *parser) decodeStringConstant(tok grammar.Token) (string, error) {
	if !p.required["encoded-character"] {
		return tok.Str, nil
	}
	s, err := grammar.DecodeEncodedCharacters(tok.Str)
	if err != nil {
		return "", fmt.Errorf("sieve: line %d: %s", tok.Line, err)
	}
	return s, nil
}

// parseCommands parses a sequence of commands. When inBlock is true it stops at
// (and requires) a closing brace; otherwise it stops at EOF.
func (p *parser) parseCommands(inBlock bool) ([]sieveCmd, error) {
	var cmds []sieveCmd
	for {
		t := p.peek()
		if t.Kind == grammar.EOF {
			if inBlock {
				return nil, p.errf("expected } but reached end of script")
			}
			return cmds, nil
		}
		if t.Kind == grammar.RBrace {
			if !inBlock {
				return nil, p.errf("unexpected }")
			}
			return cmds, nil
		}
		cmd, err := p.parseCommand()
		if err != nil {
			return nil, err
		}
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
}

func (p *parser) parseCommand() (sieveCmd, error) {
	t := p.peek()
	if t.Kind != grammar.Ident {
		return nil, p.errf("expected a command, got %s", p.describe(t))
	}
	name := p.next().Str

	// RFC 5228 §3.2: require must precede every other command. Record that a
	// non-require command has been seen so a later require is rejected.
	if name != "require" {
		p.seenNonRequire = true
	}

	switch name {
	case "require":
		return nil, p.parseRequire()
	case "if":
		return p.parseIf()
	case "elsif", "else":
		return nil, p.errf("%q without a matching if", name)
	case "stop":
		return &stopCmd{}, p.finishStatement()
	case "keep":
		return p.parseKeep()
	case "discard":
		return &discardCmd{}, p.finishStatement()
	case "fileinto":
		if err := p.requireCapability("fileinto", "the fileinto action"); err != nil {
			return nil, err
		}
		return p.parseFileinto()
	case "redirect":
		return p.parseRedirect()
	case "reject":
		if err := p.requireCapability("reject", "the reject action"); err != nil {
			return nil, err
		}
		return p.parseReject(false)
	case "ereject":
		if err := p.requireCapability("ereject", "the ereject action"); err != nil {
			return nil, err
		}
		return p.parseReject(true)
	case "setflag", "addflag", "removeflag":
		if err := p.requireCapability("imap4flags", "the "+name+" action"); err != nil {
			return nil, err
		}
		return p.parseFlag(name)
	case "vacation":
		if err := p.requireCapability("vacation", "the vacation action"); err != nil {
			return nil, err
		}
		return p.parseVacation()
	case "notify":
		if err := p.requireCapability("notify", "the notify action"); err != nil {
			return nil, err
		}
		return p.parseNotify()
	default:
		// RFC 5228 §2.10.5: a command from an extension that was not declared
		// with require (or that this package does not implement) is treated as
		// unavailable, so reject it rather than silently skipping it — a typo
		// like "kep;" must not be a silent no-op.
		return nil, p.errf("unknown command %q (extensions must be declared with require)", name)
	}
}

// requireCapability returns an error unless capability was declared with require.
// RFC 5228 §2.10.5: an extension feature that was not required MUST be treated as
// unavailable.
func (p *parser) requireCapability(capability, feature string) error {
	if !p.required[capability] {
		return p.errf("%s requires %q to be declared with require", feature, capability)
	}
	return nil
}

func (p *parser) parseRequire() error {
	// RFC 5228 §3.2: require may appear only at the start of the script, before
	// any other command. This also refuses a require inside a control block:
	// every block belongs to an if, which has already been seen.
	if p.seenNonRequire {
		return p.errf("require must appear before any other command")
	}
	names, err := p.parseStringList()
	if err != nil {
		return err
	}
	// RFC 5228 §2.10.5: requiring an extension this implementation does not
	// support is an error; the script must not run.
	for _, name := range names {
		if !supportedCapabilities[name] {
			return p.errf("require of unsupported extension %q", name)
		}
		p.required[name] = true
		if name == "vacation-seconds" {
			// RFC 6131 §2: this declaration also enables base vacation.
			p.required["vacation"] = true
		}
	}
	p.requires = append(p.requires, names...)
	return p.finishStatement()
}

func (p *parser) parseIf() (sieveCmd, error) {
	test, err := p.parseTest()
	if err != nil {
		return nil, err
	}
	block, err := p.parseBlock()
	if err != nil {
		return nil, err
	}
	branches := []sieveBranch{{test: test, block: block}}

	for p.peek().Kind == grammar.Ident && p.peek().Str == "elsif" {
		p.next()
		t, err := p.parseTest()
		if err != nil {
			return nil, err
		}
		b, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		branches = append(branches, sieveBranch{test: t, block: b})
	}

	if p.peek().Kind == grammar.Ident && p.peek().Str == "else" {
		p.next()
		b, err := p.parseBlock()
		if err != nil {
			return nil, err
		}
		branches = append(branches, sieveBranch{test: nil, block: b})
	}

	return &ifCmd{branches: branches}, nil
}

func (p *parser) parseBlock() ([]sieveCmd, error) {
	p.blockDepth++
	if p.blockDepth > p.maxDepth {
		return nil, p.errf("nesting too deep: control blocks nested more than %d levels", p.maxDepth)
	}
	defer func() { p.blockDepth-- }()

	if _, err := p.expect(grammar.LBrace, "{"); err != nil {
		return nil, err
	}
	cmds, err := p.parseCommands(true)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(grammar.RBrace, "}"); err != nil {
		return nil, err
	}
	return cmds, nil
}

// finishStatement consumes the terminating semicolon of a command.
func (p *parser) finishStatement() error {
	_, err := p.expect(grammar.Semicolon, ";")
	return err
}

// parseKeep parses keep. RFC 5228 §4.3 gives keep no arguments; the only one
// accepted is the :flags tagged argument of RFC 5232 §5, once "imap4flags" is
// declared. Anything else, such as keep "x";, is refused.
func (p *parser) parseKeep() (sieveCmd, error) {
	for p.peek().Kind == grammar.Tag {
		tag := p.next().Str
		if tag != ":flags" {
			return nil, p.errf("unexpected tagged argument %q on keep", tag)
		}
		if err := p.requireCapability("imap4flags", "keep :flags"); err != nil {
			return nil, err
		}
		if _, err := p.parseStringList(); err != nil {
			return nil, err
		}
	}
	return &keepCmd{}, p.finishStatement()
}

func (p *parser) parseFileinto() (sieveCmd, error) {
	cmd := &fileintoCmd{}
	seen := map[string]bool{}
	for p.peek().Kind == grammar.Tag {
		tag := p.next().Str
		if seen[tag] {
			return nil, p.errf("duplicate fileinto argument %q", tag)
		}
		seen[tag] = true
		switch tag {
		case ":create":
			// RFC 5490: fileinto :create requires the "mailbox" extension.
			if err := p.requireCapability("mailbox", "fileinto :create"); err != nil {
				return nil, err
			}
			cmd.create = true
		case ":flags":
			// RFC 5232: the :flags tagged argument requires "imap4flags".
			if err := p.requireCapability("imap4flags", "fileinto :flags"); err != nil {
				return nil, err
			}
			if _, err := p.parseStringList(); err != nil {
				return nil, err
			}
		case ":copy":
			// RFC 3894: the :copy tagged argument requires the "copy" extension.
			if err := p.requireCapability("copy", "fileinto :copy"); err != nil {
				return nil, err
			}
			cmd.copy = true
		default:
			return nil, p.errf("unknown fileinto argument %q", tag)
		}
	}
	folder, err := p.expect(grammar.String, "a folder name")
	if err != nil {
		return nil, err
	}
	cmd.folder = folder.Str
	return cmd, p.finishStatement()
}

func (p *parser) parseRedirect() (sieveCmd, error) {
	cmd := &redirectCmd{}
	seen := map[string]bool{}
	for p.peek().Kind == grammar.Tag {
		tag := p.next().Str
		if seen[tag] {
			return nil, p.errf("duplicate redirect argument %q", tag)
		}
		seen[tag] = true
		switch tag {
		case ":copy":
			// RFC 3894: the :copy tagged argument requires the "copy" extension.
			if err := p.requireCapability("copy", "redirect :copy"); err != nil {
				return nil, err
			}
			cmd.copy = true
		default:
			return nil, p.errf("unknown redirect argument %q", tag)
		}
	}
	addr, err := p.expect(grammar.String, "a redirect address")
	if err != nil {
		return nil, err
	}
	// RFC 5228 §2.4.2.3: it is an error to execute redirect with an argument that
	// is not a valid sieve-address. In particular a target carrying CR, LF, NUL
	// or any other control character (readily smuggled in via a "text:" literal)
	// would let a host that builds SMTP commands or message headers from the
	// address be tricked into command/header injection, so reject it here.
	if r := firstControlChar(addr.Str); r >= 0 {
		return nil, fmt.Errorf("sieve: line %d: redirect address contains a forbidden control character (%#U)", addr.Line, r)
	}
	// RFC 5228 §2.4.2.3 permits one mailbox, not a group or source route.
	// ParseAddress alone accepts a group containing one mailbox, so check the
	// delimiters too. Validate the decoded value, but preserve it for the host.
	if _, err := mail.ParseAddress(placeholderDomainLiterals(addr.Str)); err != nil {
		return nil, fmt.Errorf("sieve: line %d: invalid redirect address: %w", addr.Line, err)
	}
	if hasAddressGroupOrRoute(addr.Str) {
		return nil, fmt.Errorf("sieve: line %d: redirect address must not contain a group or source route", addr.Line)
	}
	cmd.addr = addr.Str
	return cmd, p.finishStatement()
}

// hasAddressGroupOrRoute finds group/route delimiters outside quoted strings,
// nested comments and domain literals in an already parsed mailbox. Delimiters
// in a display name, quoted localpart or IPv6 literal are ordinary data.
func hasAddressGroupOrRoute(s string) bool {
	quoted, literal := false, false
	comments := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && (quoted || comments > 0) {
			i++ // quoted-pair; syntax was checked by mail.ParseAddress.
			continue
		}
		if comments > 0 {
			switch c {
			case '(':
				comments++
			case ')':
				comments--
			}
			continue
		}
		if quoted {
			if c == '"' {
				quoted = false
			}
			continue
		}
		if literal {
			if c == ']' {
				literal = false
			}
			continue
		}
		switch c {
		case '"':
			quoted = true
		case '(':
			comments++
		case '[':
			literal = true
		case ':', ';':
			return true
		}
	}
	return false
}

// placeholderDomainLiterals replaces each well-formed domain literal outside
// quoted strings and comments with [127.0.0.1] before net/mail sees it. RFC 5322
// §3.4.1 allows any dtext in a domain literal, but net/mail accepts only IP
// literals and which IPv6 form it takes ([::1] or [IPv6:::1]) depends on the Go
// version. Checking dtext here keeps redirect validation identical everywhere.
func placeholderDomainLiterals(s string) string {
	var b strings.Builder
	quoted := false
	comments := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '\\' && (quoted || comments > 0) && i+1 < len(s) {
			b.WriteString(s[i : i+2])
			i++
			continue
		}
		switch {
		case comments > 0:
			switch c {
			case '(':
				comments++
			case ')':
				comments--
			}
		case quoted:
			quoted = c != '"'
		case c == '"':
			quoted = true
		case c == '(':
			comments++
		case c == '[':
			if end := strings.IndexByte(s[i+1:], ']'); end > 0 && isDtext(s[i+1:i+1+end]) {
				b.WriteString("[127.0.0.1]")
				i += end + 1
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// isDtext reports whether s is non-empty RFC 5322 dtext: printable US-ASCII
// other than "[", "]" and "\".
func isDtext(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 33 || c > 126 || c == '[' || c == ']' || c == '\\' {
			return false
		}
	}
	return s != ""
}

// firstControlChar returns the first control character (C0/C1, including CR, LF,
// TAB and NUL) in s, or -1 if s contains none.
func firstControlChar(s string) rune {
	for _, r := range s {
		if unicode.IsControl(r) {
			return r
		}
	}
	return -1
}

// parseReject parses reject and ereject. RFC 5429 §2.1 and §2.2 give both a
// single required argument, reject <reason: string>, so a bare reject; is
// refused.
func (p *parser) parseReject(extended bool) (sieveCmd, error) {
	reason, err := p.expect(grammar.String, "a reason string")
	if err != nil {
		return nil, err
	}
	return &rejectCmd{reason: reason.Str, extended: extended}, p.finishStatement()
}

func (p *parser) parseFlag(op string) (sieveCmd, error) {
	// imap4flags setflag/addflag/removeflag may take an optional variable name
	// (which we do not support) followed by the flag list. We only accept the
	// flag string / string-list form.
	flags, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	return &flagCmd{op: op, flags: normalizeFlags(flags)}, p.finishStatement()
}

// settableSystemFlags are the IMAP system flags a client may set (RFC 3501
// §2.3.2); \Recent and any other system flag are not.
var settableSystemFlags = map[string]string{
	`\answered`: `\Answered`,
	`\flagged`:  `\Flagged`,
	`\deleted`:  `\Deleted`,
	`\seen`:     `\Seen`,
	`\draft`:    `\Draft`,
}

// normalizeFlags applies RFC 5232 §2 to a list-of-flags: each string is a
// space-separated list of flag names, empty names are ignored, and a name that
// is not a valid, settable IMAP flag is dropped. Flags are compared
// case-insensitively, so a repeated flag is kept once (§3 allows it).
func normalizeFlags(list []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, s := range list {
		for _, f := range strings.Split(s, " ") {
			if strings.HasPrefix(f, `\`) {
				var ok bool
				if f, ok = settableSystemFlags[strings.ToLower(f)]; !ok {
					continue
				}
			} else if !isIMAPAtom(f) {
				continue
			}
			key := strings.ToLower(f)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}

// isIMAPAtom reports whether s is an IMAP atom (RFC 3501 §9): one or more
// ASCII characters, none of them a control character, a space or one of
// ( ) { % * " \ ].
func isIMAPAtom(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= 0x20 || c >= 0x7f || strings.IndexByte(`(){%*"\]`, c) >= 0 {
			return false
		}
	}
	return true
}

// Vacation days use a site minimum of one day and maximum of 365 days
// (RFC 5230 §4.1); seconds use 0..2147483647 (RFC 6131 §2). Clamp before
// converting to Duration to avoid overflow.
func (p *parser) parseVacation() (sieveCmd, error) {
	cmd := &vacationCmd{interval: 7 * 24 * time.Hour}
	seen := map[string]bool{}
	for p.peek().Kind == grammar.Tag {
		tag := p.next().Str
		if seen[tag] {
			return nil, p.errf("duplicate vacation argument %q", tag)
		}
		seen[tag] = true
		if seen[":days"] && seen[":seconds"] {
			return nil, p.errf("vacation :days and :seconds are mutually exclusive")
		}
		switch tag {
		case ":days":
			n, err := p.expect(grammar.Number, "a day count")
			if err != nil {
				return nil, err
			}
			days := min(max(n.Num, 1), 365)
			cmd.interval = time.Duration(days) * 24 * time.Hour
		case ":seconds":
			if err := p.requireCapability("vacation-seconds", "vacation :seconds"); err != nil {
				return nil, err
			}
			n, err := p.expect(grammar.Number, "a seconds count")
			if err != nil {
				return nil, err
			}
			// RFC 6131 §2 permits a site maximum of at least one day.
			// Accept all representable Sieve numbers, capping before duration
			// conversion so a huge count cannot wrap to zero or negative.
			cmd.interval = time.Duration(min(n.Num, 1<<31-1)) * time.Second
		case ":subject", ":from", ":handle":
			s, err := p.expect(grammar.String, "a string")
			if err != nil {
				return nil, err
			}
			switch tag {
			case ":subject":
				cmd.subject, cmd.subjectSet = s.Str, true
			case ":from":
				if _, err := mail.ParseAddressList(s.Str); err != nil {
					return nil, p.errf("invalid vacation :from mailbox-list: %v", err)
				}
				cmd.from = s.Str
			case ":handle":
				cmd.handle, cmd.handleSet = s.Str, true
			}
		case ":addresses":
			addresses, err := p.parseStringList()
			if err != nil {
				return nil, err
			}
			cmd.addresses = addresses
		case ":mime":
			cmd.mime = true
		default:
			return nil, p.errf("unknown vacation argument %q", tag)
		}
	}
	body, err := p.expect(grammar.String, "a vacation reason")
	if err != nil {
		return nil, err
	}
	cmd.body = body.Str
	return cmd, p.finishStatement()
}

func (p *parser) parseNotify() (sieveCmd, error) {
	cmd := &notifyCmd{}
	for p.peek().Kind == grammar.Tag {
		switch p.next().Str {
		case ":method":
			s, err := p.expect(grammar.String, "a notification method")
			if err != nil {
				return nil, err
			}
			cmd.method = s.Str
		case ":message":
			s, err := p.expect(grammar.String, "a message")
			if err != nil {
				return nil, err
			}
			cmd.message = s.Str
		case ":options":
			if _, err := p.parseStringList(); err != nil {
				return nil, err
			}
		case ":from", ":importance", ":id":
			if _, err := p.expect(grammar.String, "a string"); err != nil {
				return nil, err
			}
		}
	}
	// RFC 5435 puts the method in a trailing positional string.
	if cmd.method == "" && p.peek().Kind == grammar.String {
		method, err := p.decodeStringConstant(p.next())
		if err != nil {
			return nil, err
		}
		cmd.method = method
	}
	return cmd, p.finishStatement()
}

// ── Test parsing ─────────────────────────────────────────────────────

func (p *parser) parseTest() (sieveTest, error) {
	p.testDepth++
	if p.testDepth > p.maxDepth {
		return nil, p.errf("nesting too deep: test expressions nested more than %d levels", p.maxDepth)
	}
	defer func() { p.testDepth-- }()

	t := p.peek()
	if t.Kind != grammar.Ident {
		return nil, p.errf("expected a test, got %s", p.describe(t))
	}
	name := p.next().Str

	switch name {
	case "allof":
		tests, err := p.parseTestList()
		if err != nil {
			return nil, err
		}
		return &allofTest{tests: tests}, nil
	case "anyof":
		tests, err := p.parseTestList()
		if err != nil {
			return nil, err
		}
		return &anyofTest{tests: tests}, nil
	case "not":
		inner, err := p.parseTest()
		if err != nil {
			return nil, err
		}
		return &notTest{inner: inner}, nil
	case "true":
		return &boolTest{val: true}, nil
	case "false":
		return &boolTest{val: false}, nil
	case "exists":
		headers, err := p.parseStringList()
		if err != nil {
			return nil, err
		}
		return &existsTest{headers: headers}, nil
	case "size":
		return p.parseSizeTest()
	case "header":
		return p.parseHeaderTest()
	case "address":
		return p.parseAddressTest()
	case "envelope":
		if err := p.requireCapability("envelope", "the envelope test"); err != nil {
			return nil, err
		}
		return p.parseEnvelopeTest()
	case "body":
		if err := p.requireCapability("body", "the body test"); err != nil {
			return nil, err
		}
		return p.parseBodyTest()
	default:
		// RFC 5228 §2.10.5: a test from an extension that was not declared with
		// require (or that this package does not implement) is unavailable, so
		// reject it rather than silently treating it as false.
		return nil, p.errf("unknown test %q (extensions must be declared with require)", name)
	}
}

func (p *parser) parseTestList() ([]sieveTest, error) {
	if _, err := p.expect(grammar.LParen, "("); err != nil {
		return nil, err
	}
	var tests []sieveTest
	for {
		t, err := p.parseTest()
		if err != nil {
			return nil, err
		}
		tests = append(tests, t)
		if p.peek().Kind == grammar.Comma {
			p.next()
			continue
		}
		break
	}
	if _, err := p.expect(grammar.RParen, ")"); err != nil {
		return nil, err
	}
	return tests, nil
}

func (p *parser) parseSizeTest() (sieveTest, error) {
	st := &sizeTest{}
	gotOp := false
	for p.peek().Kind == grammar.Tag {
		tag := p.next().Str
		switch tag {
		case ":over", ":under":
			// RFC 5228 §5.9: size takes exactly one of :over / :under. A second
			// relation (whether the same tag repeated or the opposing one) is a
			// conflict, not a last-wins overwrite.
			if gotOp {
				return nil, p.errf("size test takes exactly one of :over or :under, not a further %q", tag)
			}
			st.over = tag == ":over"
			gotOp = true
		default:
			return nil, p.errf("unexpected tagged argument %q on the size test", tag)
		}
	}
	if !gotOp {
		return nil, p.errf("size test requires :over or :under")
	}
	n, err := p.expect(grammar.Number, "a size limit")
	if err != nil {
		return nil, err
	}
	st.limit = n.Num
	return st, nil
}

// matchOptions collects the tagged arguments shared by string-matching tests.
type matchOptions struct {
	comparator    string
	matchType     string
	addressPart   string
	bodyTransform string
	contentTypes  []string
}

// parseMatchOptions consumes comparator, match-type and (for address/envelope)
// address-part tags in any order. allowAddressPart selects whether
// :localpart/:domain/:all are recognised; allowBody selects the body-transform
// tags :raw/:text/:content.
//
// RFC 5228 §2.7.1 (a test carries a single match type), §2.7.3 (a single
// comparator) and §2.7.4/§5.7 (address parts apply only to address/envelope, and
// a test carries a single one) make a duplicated or conflicting tagged argument
// a syntax error, not a last-wins overwrite. A tagged argument that is not
// defined for the test (an address part on header/body, a body transform on a
// non-body test, or any foreign tag) is likewise rejected rather than silently
// consumed. Each mutually-exclusive group — match type, comparator, address
// part, body transform — may therefore appear at most once.
func (p *parser) parseMatchOptions(allowAddressPart, allowBody bool) (matchOptions, error) {
	opts := matchOptions{
		comparator:    defaultComparator,
		matchType:     defaultMatchType,
		addressPart:   defaultAddressPart,
		bodyTransform: defaultBodyTransform,
	}
	var matchTypeSet, comparatorSet, addressPartSet, bodyTransformSet bool
	for p.peek().Kind == grammar.Tag {
		tag := p.next().Str
		switch tag {
		case ":is", ":contains", ":matches":
			if matchTypeSet {
				return opts, p.errf("duplicate or conflicting match type %q: a test takes at most one match type", tag)
			}
			matchTypeSet = true
			opts.matchType = tag
		case ":regex":
			// The :regex match type is a non-standard extension; it must be
			// declared with require "regex" before use.
			if err := p.requireCapability("regex", "the :regex match type"); err != nil {
				return opts, err
			}
			if matchTypeSet {
				return opts, p.errf("duplicate or conflicting match type %q: a test takes at most one match type", tag)
			}
			matchTypeSet = true
			opts.matchType = tag
		case ":comparator":
			if comparatorSet {
				return opts, p.errf("duplicate :comparator: a test takes at most one comparator")
			}
			s, err := p.expect(grammar.String, "a comparator name")
			if err != nil {
				return opts, err
			}
			comparatorSet = true
			// RFC 4790 §3.1 restricts collation identifiers to US-ASCII.
			// Unicode lowercasing would accept dotted I as an ASCII i.
			opts.comparator = match.ASCIIFold(s.Str)
			// RFC 4790/5228: only i;ascii-casemap and i;octet are built in; any
			// other comparator (e.g. i;ascii-numeric) needs a matching require.
			if opts.comparator != defaultComparator && opts.comparator != "i;octet" {
				if err := p.requireCapability("comparator-"+opts.comparator, "the "+opts.comparator+" comparator"); err != nil {
					return opts, err
				}
			}
		case ":localpart", ":domain", ":all":
			if !allowAddressPart {
				return opts, p.errf("tagged argument %q is not valid on this test (address parts apply only to address and envelope tests)", tag)
			}
			if addressPartSet {
				return opts, p.errf("duplicate or conflicting address part %q: a test takes at most one address part", tag)
			}
			addressPartSet = true
			opts.addressPart = tag
		case ":raw", ":text", ":content":
			if !allowBody {
				return opts, p.errf("tagged argument %q is not valid on this test (body transforms apply only to the body test)", tag)
			}
			if bodyTransformSet {
				return opts, p.errf("duplicate or conflicting body transform %q: a test takes at most one body transform", tag)
			}
			bodyTransformSet = true
			opts.bodyTransform = tag
			// :content carries a string list of content types (RFC 5173 §5).
			if tag == ":content" {
				types, err := p.parseStringList()
				if err != nil {
					return opts, err
				}
				opts.contentTypes = types
			}
		default:
			return opts, p.errf("unexpected tagged argument %q on this test", tag)
		}
	}
	if err := checkComparatorMatchType(opts.comparator, opts.matchType); err != nil {
		return opts, p.errf("%s", err)
	}
	return opts, nil
}

// checkComparatorMatchType rejects a comparator used with a match type it does
// not define. A comparator is not a free substitute across every match type
// (RFC 5228 §2.7.3): each comparator declares which operations it supports, and
// using one with an unsupported operation is an error rather than a silent
// fall-back to the default comparator.
//
// Of the three comparators this package implements, only i;ascii-numeric is
// restricted. RFC 4790 §9.1 defines it purely for equality and ordering of
// decimal integers and states it has no substring operation, so it cannot back
// the substring match types :contains and :matches (RFC 5228 §2.7.1) or the
// non-standard :regex match type — the numeric interpretation is meaningless
// for a substring or wildcard/regex pattern. It therefore applies only to the
// equality match type :is here. (An ordering use would require the relational
// extension's :value/:count, RFC 5231, which this package does not implement.)
// i;octet and i;ascii-casemap support every match type and are unrestricted.
func checkComparatorMatchType(comparator, matchType string) error {
	if comparator == "i;ascii-numeric" {
		switch matchType {
		case ":contains", ":matches", ":regex":
			return fmt.Errorf("the i;ascii-numeric comparator does not support the %s match type: it has only equality and ordering, no substring operation (RFC 4790 §9.1)", matchType)
		}
	}
	return nil
}

func (p *parser) parseHeaderTest() (sieveTest, error) {
	opts, err := p.parseMatchOptions(false, false)
	if err != nil {
		return nil, err
	}
	headers, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	keys, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	return &headerTest{
		comparator: opts.comparator,
		matchType:  opts.matchType,
		headers:    headers,
		keys:       keys,
	}, nil
}

func (p *parser) parseAddressTest() (sieveTest, error) {
	opts, err := p.parseMatchOptions(true, false)
	if err != nil {
		return nil, err
	}
	headers, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	keys, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	return &addressTest{
		comparator:  opts.comparator,
		matchType:   opts.matchType,
		addressPart: opts.addressPart,
		headers:     headers,
		keys:        keys,
	}, nil
}

func (p *parser) parseEnvelopeTest() (sieveTest, error) {
	opts, err := p.parseMatchOptions(true, false)
	if err != nil {
		return nil, err
	}
	parts, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	// RFC 5228 §5.4 recommends an error for unknown envelope parts. Check
	// every decoded operand now, even in a branch that will not be evaluated.
	for _, part := range parts {
		switch match.ASCIIFold(part) {
		case "from", "to":
		default:
			return nil, p.errf("unknown envelope part %q", part)
		}
	}
	keys, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	return &envelopeTest{
		comparator:  opts.comparator,
		matchType:   opts.matchType,
		addressPart: opts.addressPart,
		parts:       parts,
		keys:        keys,
	}, nil
}

func (p *parser) parseBodyTest() (sieveTest, error) {
	opts, err := p.parseMatchOptions(false, true)
	if err != nil {
		return nil, err
	}
	keys, err := p.parseStringList()
	if err != nil {
		return nil, err
	}
	return &bodyTest{
		comparator:   opts.comparator,
		matchType:    opts.matchType,
		transform:    opts.bodyTransform,
		contentTypes: opts.contentTypes,
		keys:         keys,
	}, nil
}

// parseStringList parses either a single quoted string or a bracketed,
// comma-separated list of strings. Per RFC 5228 §8.2 the string-list grammar is
// `"[" string *("," string) "]" / string`, so a bracketed list must contain at
// least one string; an empty "[]" has no production and is rejected. This keeps
// an empty list from being silently accepted where the grammar requires a value
// (e.g. `exists []`, which would otherwise evaluate true vacuously).
func (p *parser) parseStringList() ([]string, error) {
	if p.peek().Kind == grammar.String {
		s, err := p.decodeStringConstant(p.next())
		if err != nil {
			return nil, err
		}
		return []string{s}, nil
	}
	open, err := p.expect(grammar.LBracket, "a string or string list")
	if err != nil {
		return nil, err
	}
	var list []string
	for p.peek().Kind != grammar.RBracket {
		s, err := p.expect(grammar.String, "a string")
		if err != nil {
			return nil, err
		}
		list = append(list, s.Str)
		if p.peek().Kind == grammar.Comma {
			p.next()
			// RFC 5228 §8.1: string-list = "[" string *("," string) "]", so a
			// comma must be followed by another string.
			if p.peek().Kind == grammar.RBracket {
				return nil, p.errf("expected a string after the comma, got ]")
			}
			continue
		}
		break
	}
	if _, err := p.expect(grammar.RBracket, "]"); err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, fmt.Errorf("sieve: line %d: empty string list []: a string list must contain at least one string", open.Line)
	}
	return list, nil
}

// ── encoded-character (RFC 5228 §2.4.2.4) ────────────────────────────
