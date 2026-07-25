// Package sieve parses and executes scripts written in the Sieve mail-filtering
// language (RFC 5228) — the language mail servers use to let users sort, file,
// redirect, and auto-reply to incoming mail.
//
// Parsing and evaluation are decoupled from any mailbox model. A caller [Parse]s
// a script once into a [Script], adapts its own email into the neutral [Message]
// type, and calls [Script.Evaluate] with an [Executor] it implements. The
// evaluator walks the script, evaluates each test against the message, and calls
// the Executor's methods to apply the actions the script selected (fileinto,
// redirect, imap4flags, vacation, notify, keep). The two terminal dispositions,
// discard and reject, are reported through the returned [Outcome] rather than
// the Executor. How each action maps onto a real mailbox — folder names, flag
// storage, vacation de-duplication, notification transport — is entirely the
// Executor's concern: this package decides what to do, the host decides how. It
// depends only on the Go standard library.
//
// # Evaluating a script
//
//	script, err := sieve.Parse(src)
//	if err != nil {
//		// syntax error
//	}
//	outcome := script.Evaluate(msg, exec) // exec implements sieve.Executor
//	switch outcome.Disposition {
//	case sieve.Discard:
//		// silently drop the message
//	case sieve.Reject:
//		// refuse it, citing outcome.RejectReason
//	default:
//		// deliver, honouring the actions exec recorded
//	}
//
// Use [Validate] to check a script's syntax without evaluating it.
//
// # Supported language
//
// Control commands: if / elsif / else, require, stop.
//
// Tests: address, header, envelope, exists, size (:over / :under), body, allof,
// anyof, not, true / false.
//
// Actions: keep, discard, fileinto (with :create), redirect, reject, imap4flags
// (setflag / addflag / removeflag), vacation, notify.
//
// Match types: :is, :contains, :matches (glob with * and ?), and a non-standard
// :regex extension. Comparators: i;ascii-casemap (the default), i;octet, and
// i;ascii-numeric. Address parts: :all, :localpart, :domain.
//
// # Unknown extensions
//
// Parsing is strict about the constructs it understands, so [Parse] and
// [Validate] catch real mistakes, but lenient about the rest: unknown commands,
// tests, and tagged arguments are skipped, so a script that uses an extension
// this package does not implement still loads and runs its recognised parts.
// [Script.Requires] reports the extensions the script declared via "require".
package sieve

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Message is the neutral view of an email the evaluator tests against. A host
// maps its own representation onto this type before evaluating; the evaluator
// never sees the host's message model.
type Message struct {
	Headers     Headers
	Envelope    Envelope
	Body        Body
	Attachments []Attachment
}

// Headers holds the structured and raw headers used by header/address/exists
// tests. Raw is consulted (case-insensitively) for any header not covered by a
// structured field, so custom headers such as X-Priority are testable.
type Headers struct {
	Subject    string
	From       []Address
	To         []Address
	Cc         []Address
	Bcc        []Address
	MessageID  string
	InReplyTo  string
	Date       string
	References []string
	Raw        map[string][]string
}

// Address is a structured email address.
type Address struct {
	Name    string
	Address string
}

// Envelope holds the SMTP envelope identities used by the envelope test. The
// host resolves any override (e.g. a gateway-supplied sender) before populating
// these fields.
type Envelope struct {
	From string
	To   []string
}

// Body is a (possibly multipart) message body used by the body test and size.
type Body struct {
	ContentType string
	Content     string
	Parts       []Body
}

// Attachment contributes its octet Size to the size test.
type Attachment struct {
	Size int64
}

// Disposition is the terminal delivery decision an evaluation reached.
type Disposition int

const (
	// Continue means no terminal action fired; deliver honouring whatever
	// actions the Executor recorded.
	Continue Disposition = iota
	// Discard means the script asked to silently drop the message.
	Discard
	// Reject means the script asked to refuse the message (see Outcome.RejectReason).
	Reject
)

// Outcome is the result of evaluating a script.
type Outcome struct {
	Disposition  Disposition
	RejectReason string // set only when Disposition == Reject
}

// Vacation carries the arguments of a matched vacation action (RFC 5230). The
// evaluator computes ReplyTo (the envelope sender, falling back to the From
// header); the Executor is responsible for de-duplication and for actually
// sending the auto-reply.
type Vacation struct {
	Days    int
	Subject string
	Body    string
	ReplyTo string
}

// Executor applies the (non-terminal) actions a Sieve script selects. The host
// implements it to map each action onto its mailbox/delivery model. Methods are
// invoked in script order; the terminal actions discard and reject are not
// Executor methods but are reported via the [Outcome] returned by
// [Script.Evaluate].
type Executor interface {
	// Keep requests explicit delivery to the default mailbox.
	Keep()
	// FileInto delivers into the named folder, creating it when create is set.
	FileInto(folder string, create bool)
	// Redirect forwards the message to addr.
	Redirect(addr string)
	// Flag applies an imap4flags operation ("setflag", "addflag", "removeflag").
	Flag(op string, flags []string)
	// Vacation records a vacation auto-reply request.
	Vacation(v Vacation)
	// Notify records a notification request.
	Notify(method, message string)
}

// Validate reports whether a Sieve script is syntactically valid. It accepts
// the same [Option]s as [Parse]; nesting beyond [DefaultMaxDepth] (or the limit
// set with [WithMaxDepth]) is reported as invalid.
func Validate(script string, opts ...Option) error {
	if _, err := parseSieveScript(script, opts...); err != nil {
		return fmt.Errorf("invalid sieve script: %w", err)
	}
	return nil
}

// Evaluate runs the script against msg, invoking exec for each action it
// selects, and returns the terminal [Outcome].
func (s *Script) Evaluate(msg *Message, exec Executor) Outcome {
	if s == nil {
		return Outcome{Disposition: Continue}
	}
	st := &evalState{msg: msg, exec: exec}
	out, _ := st.runBlock(s.commands)
	return out
}

// evalState carries the immutable inputs while walking a script's commands.
type evalState struct {
	msg  *Message
	exec Executor
}

// runBlock evaluates a sequence of commands. The bool result reports whether
// evaluation should halt (a terminal discard/reject, or a stop).
func (st *evalState) runBlock(cmds []sieveCmd) (Outcome, bool) {
	for _, c := range cmds {
		out, halt := st.runCmd(c)
		if halt {
			return out, true
		}
	}
	return Outcome{Disposition: Continue}, false
}

func (st *evalState) runCmd(c sieveCmd) (Outcome, bool) {
	switch cmd := c.(type) {
	case *ifCmd:
		for _, br := range cmd.branches {
			if br.test == nil || evalTest(br.test, st.msg) {
				return st.runBlock(br.block)
			}
		}

	case *stopCmd:
		// Halt further Sieve processing but still deliver per actions so far.
		return Outcome{Disposition: Continue}, true

	case *keepCmd:
		st.exec.Keep()

	case *discardCmd:
		return Outcome{Disposition: Discard}, true

	case *rejectCmd:
		return Outcome{Disposition: Reject, RejectReason: cmd.reason}, true

	case *fileintoCmd:
		st.exec.FileInto(cmd.folder, cmd.create)

	case *redirectCmd:
		st.exec.Redirect(cmd.addr)

	case *flagCmd:
		st.exec.Flag(cmd.op, cmd.flags)

	case *vacationCmd:
		st.exec.Vacation(Vacation{
			Days:    cmd.days,
			Subject: cmd.subject,
			Body:    cmd.body,
			ReplyTo: st.vacationReplyTo(),
		})

	case *notifyCmd:
		st.exec.Notify(cmd.method, cmd.message)
	}

	return Outcome{Disposition: Continue}, false
}

// vacationReplyTo picks the address a vacation auto-reply should go to: the
// envelope sender, falling back to the first From address.
func (st *evalState) vacationReplyTo() string {
	replyTo := st.msg.Envelope.From
	if replyTo == "" && len(st.msg.Headers.From) > 0 {
		replyTo = st.msg.Headers.From[0].Address
	}
	return replyTo
}

// ── Test evaluation ──────────────────────────────────────────────────

func evalTest(t sieveTest, msg *Message) bool {
	switch tt := t.(type) {
	case *allofTest:
		for _, sub := range tt.tests {
			if !evalTest(sub, msg) {
				return false
			}
		}
		return true
	case *anyofTest:
		for _, sub := range tt.tests {
			if evalTest(sub, msg) {
				return true
			}
		}
		return false
	case *notTest:
		return !evalTest(tt.inner, msg)
	case *boolTest:
		return tt.val
	case *existsTest:
		for _, h := range tt.headers {
			if len(headerValues(msg, h)) == 0 {
				return false
			}
		}
		return true
	case *sizeTest:
		size := messageSize(msg)
		if tt.over {
			return size > tt.limit
		}
		return size < tt.limit
	case *headerTest:
		values := gatherHeaderValues(msg, tt.headers)
		return matchAny(values, tt.matchType, tt.comparator, tt.keys)
	case *addressTest:
		values := gatherAddressValues(msg, tt.headers, tt.addressPart)
		return matchAny(values, tt.matchType, tt.comparator, tt.keys)
	case *envelopeTest:
		values := gatherEnvelopeValues(msg, tt.parts, tt.addressPart)
		return matchAny(values, tt.matchType, tt.comparator, tt.keys)
	case *bodyTest:
		return matchAny([]string{extractBodyText(msg)}, tt.matchType, tt.comparator, tt.keys)
	}
	return false
}

// messageSize approximates the octet size of the message: body content, nested
// part content, and known attachment sizes.
func messageSize(msg *Message) int64 {
	var size int64
	size += bodySize(msg.Body)
	for _, a := range msg.Attachments {
		size += a.Size
	}
	return size
}

func bodySize(b Body) int64 {
	size := int64(len(b.Content))
	for _, p := range b.Parts {
		size += bodySize(p)
	}
	return size
}

// ── Value extraction ─────────────────────────────────────────────────

func gatherHeaderValues(msg *Message, names []string) []string {
	var out []string
	for _, n := range names {
		out = append(out, headerValues(msg, n)...)
	}
	return out
}

// headerValues returns every value of the named header, drawing from both the
// structured Headers fields and the raw header map (matched case-insensitively).
func headerValues(msg *Message, name string) []string {
	lower := strings.ToLower(strings.TrimSpace(name))
	var out []string
	switch lower {
	case "subject":
		if msg.Headers.Subject != "" {
			out = append(out, msg.Headers.Subject)
		}
	case "from":
		out = append(out, formatAddresses(msg.Headers.From)...)
	case "to":
		out = append(out, formatAddresses(msg.Headers.To)...)
	case "cc":
		out = append(out, formatAddresses(msg.Headers.Cc)...)
	case "bcc":
		out = append(out, formatAddresses(msg.Headers.Bcc)...)
	case "message-id":
		if msg.Headers.MessageID != "" {
			out = append(out, msg.Headers.MessageID)
		}
	case "in-reply-to":
		if msg.Headers.InReplyTo != "" {
			out = append(out, msg.Headers.InReplyTo)
		}
	case "date":
		if msg.Headers.Date != "" {
			out = append(out, msg.Headers.Date)
		}
	case "references":
		out = append(out, msg.Headers.References...)
	}
	for k, vals := range msg.Headers.Raw {
		if strings.EqualFold(k, lower) {
			out = append(out, vals...)
		}
	}
	return out
}

// formatAddresses renders structured addresses as header-style values.
func formatAddresses(addrs []Address) []string {
	var out []string
	for _, a := range addrs {
		switch {
		case a.Name != "" && a.Address != "":
			out = append(out, a.Name+" <"+a.Address+">")
		case a.Address != "":
			out = append(out, a.Address)
		case a.Name != "":
			out = append(out, a.Name)
		}
	}
	return out
}

func gatherAddressValues(msg *Message, names []string, part string) []string {
	var out []string
	for _, n := range names {
		for _, addr := range addressList(msg, n) {
			out = append(out, addressPartOf(addr, part))
		}
	}
	return out
}

// addressList returns the bare addr-specs of a header (no display name).
func addressList(msg *Message, name string) []string {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch lower {
	case "from":
		return addressSpecs(msg.Headers.From)
	case "to":
		return addressSpecs(msg.Headers.To)
	case "cc":
		return addressSpecs(msg.Headers.Cc)
	case "bcc":
		return addressSpecs(msg.Headers.Bcc)
	}
	var out []string
	for k, vals := range msg.Headers.Raw {
		if strings.EqualFold(k, lower) {
			out = append(out, vals...)
		}
	}
	return out
}

func addressSpecs(addrs []Address) []string {
	var out []string
	for _, a := range addrs {
		if a.Address != "" {
			out = append(out, a.Address)
		}
	}
	return out
}

func gatherEnvelopeValues(msg *Message, parts []string, part string) []string {
	var out []string
	for _, name := range parts {
		for _, v := range envelopeValues(msg, name) {
			out = append(out, addressPartOf(v, part))
		}
	}
	return out
}

func envelopeValues(msg *Message, name string) []string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "from":
		if msg.Envelope.From != "" {
			return []string{msg.Envelope.From}
		}
	case "to":
		return msg.Envelope.To
	}
	return nil
}

// addressPartOf extracts the requested part (:all/:localpart/:domain) from an
// address. For an address without an "@", the whole string is the local part
// and the domain is empty.
func addressPartOf(addr, part string) string {
	switch part {
	case ":localpart":
		if i := strings.LastIndex(addr, "@"); i >= 0 {
			return addr[:i]
		}
		return addr
	case ":domain":
		if i := strings.LastIndex(addr, "@"); i >= 0 {
			return addr[i+1:]
		}
		return ""
	default: // :all
		return addr
	}
}

// ── Matching ─────────────────────────────────────────────────────────

// matchAny reports whether any value matches any key under the given match
// type and comparator.
func matchAny(values []string, matchType, comparator string, keys []string) bool {
	for _, v := range values {
		for _, k := range keys {
			if matchOne(v, k, matchType, comparator) {
				return true
			}
		}
	}
	return false
}

func matchOne(value, key, matchType, comparator string) bool {
	switch matchType {
	case ":is":
		return compareIs(value, key, comparator)
	case ":contains":
		if comparator == "i;octet" {
			return strings.Contains(value, key)
		}
		return strings.Contains(strings.ToLower(value), strings.ToLower(key))
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
		return strings.Contains(strings.ToLower(value), strings.ToLower(key))
	}
}

func compareIs(value, key, comparator string) bool {
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
		return strings.EqualFold(value, key)
	}
}

// asciiNumeric parses a leading run of digits per the i;ascii-numeric
// comparator (RFC 4790 §9.1). Returns false when the value does not begin with
// a digit.
func asciiNumeric(s string) (uint64, bool) {
	s = strings.TrimSpace(s)
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, false
	}
	n, err := strconv.ParseUint(s[:end], 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func foldForComparator(s, comparator string) string {
	if comparator == "i;octet" {
		return s
	}
	return strings.ToLower(s)
}

// wildcardMatch implements Sieve :matches semantics: '*' matches zero or more
// characters, '?' matches exactly one, and a backslash escapes the next
// character. It compiles the pattern to an anchored regular expression.
func wildcardMatch(value, pattern string) bool {
	var b strings.Builder
	b.WriteString(`\A`)
	runes := []rune(pattern)
	for i := 0; i < len(runes); i++ {
		switch runes[i] {
		case '\\':
			if i+1 < len(runes) {
				b.WriteString(regexp.QuoteMeta(string(runes[i+1])))
				i++
			} else {
				b.WriteString(regexp.QuoteMeta(`\`))
			}
		case '*':
			b.WriteString(`(?s:.*)`)
		case '?':
			b.WriteString(`(?s:.)`)
		default:
			b.WriteString(regexp.QuoteMeta(string(runes[i])))
		}
	}
	b.WriteString(`\z`)
	re, err := regexp.Compile(b.String())
	if err != nil {
		return false
	}
	return re.MatchString(value)
}

// ── Body text extraction ─────────────────────────────────────────────

// extractBodyText returns the plain text content of the email body.
// It prefers text/plain parts; falls back to stripping HTML tags from text/html.
func extractBodyText(msg *Message) string {
	// Try top-level body first.
	if msg.Body.Content != "" {
		ct := strings.ToLower(msg.Body.ContentType)
		if strings.HasPrefix(ct, "text/plain") || ct == "" {
			return msg.Body.Content
		}
		if strings.HasPrefix(ct, "text/html") {
			return stripHTMLTags(msg.Body.Content)
		}
	}

	// Search parts for text/plain first, then text/html.
	if plain := findPartContent(msg.Body.Parts, "text/plain"); plain != "" {
		return plain
	}
	if html := findPartContent(msg.Body.Parts, "text/html"); html != "" {
		return stripHTMLTags(html)
	}

	// Fallback: return raw content.
	return msg.Body.Content
}

// findPartContent recursively searches body parts for a matching content type
// and returns the first match's content.
func findPartContent(parts []Body, contentType string) string {
	for _, p := range parts {
		if strings.HasPrefix(strings.ToLower(p.ContentType), contentType) && p.Content != "" {
			return p.Content
		}
		if found := findPartContent(p.Parts, contentType); found != "" {
			return found
		}
	}
	return ""
}

// stripHTMLTags removes HTML tags from a string for plain-text matching.
// This is a simplified implementation, not a full HTML parser.
func stripHTMLTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			b.WriteByte(' ') // replace tag with space
		case !inTag:
			b.WriteRune(r)
		}
	}
	return b.String()
}
