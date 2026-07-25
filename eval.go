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
//		// deliver, honouring the actions exec recorded; additionally deliver to
//		// the default mailbox when outcome.ImplicitKeep is set (the RFC 5228
//		// §2.10.2 implicit keep)
//	}
//
// The evaluator applies the RFC 5228 §2.10.2 implicit keep: unless the script
// cancels it with keep, fileinto, redirect, or discard, the message is delivered
// to the default mailbox, signalled by [Outcome.ImplicitKeep]. A discard only
// cancels that keep — it does not stop the script, so later actions still run.
// A runtime error during evaluation fails safe to the implicit keep (§2.10.6),
// reported via [Outcome.Error], rather than losing the message.
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
// # require and extensions
//
// Parsing enforces RFC 5228 "require" semantics. Every extension a script uses
// must be declared with require before use; requiring an extension this package
// does not implement is an error; require must precede every other command; and
// an unknown command or test (including a typo) is a parse error rather than a
// silent no-op. [Parse] and [Validate] therefore reject a script that uses an
// unsupported or undeclared extension instead of running it partially.
// [Script.Requires] reports the extensions the script declared via "require".
package sieve

import (
	"fmt"
	"net/mail"
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
	// RawSize is the exact octet count of the whole message as it appears on
	// the wire — the header block, the blank line separating headers from body,
	// and the body. The size test (RFC 5228 §5.9) counts the entire message, so
	// when the host knows this figure it should set RawSize and it is used
	// verbatim. When RawSize is zero the size is reconstructed from the Headers,
	// Body, and Attachments fields, which is only an approximation because the
	// reconstructed headers need not be byte-identical to the original.
	RawSize int64
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
	// From is the SMTP reverse-path (MAIL FROM) address. An empty From with
	// FromNull unset means the envelope sender is absent (no MAIL FROM was
	// seen); the envelope "from" test then produces no value to match.
	From string
	// FromNull marks a null reverse-path (SMTP "MAIL FROM:<>", i.e. a bounce).
	// Per RFC 5228 §5.4 the envelope "from" value is then the empty string, so
	// `envelope :is "from" ""` matches. Set it (with From left empty) to
	// distinguish a genuine null reverse-path from a merely absent sender.
	FromNull bool
	To       []string
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
	// actions the Executor recorded, plus the implicit keep to the default
	// mailbox when Outcome.ImplicitKeep is set.
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
	// ImplicitKeep reports that the RFC 5228 §2.10.2 implicit keep is in effect:
	// the script cancelled no keep and performed no fileinto, redirect, or
	// discard, so the host must deliver the message to the default mailbox in
	// addition to honouring any actions the Executor recorded. It is only ever set
	// alongside Disposition == Continue. It is also set on the §2.10.6 fail-safe
	// keep (see Error). When a delivering action ran, or a discard dropped the
	// message, ImplicitKeep is false.
	ImplicitKeep bool
	// Error is non-nil when evaluation was aborted by a runtime error — for
	// example an Executor callback panicked. Per RFC 5228 §2.10.6 the message is
	// then kept rather than lost (ImplicitKeep is true and Disposition is
	// Continue) and the host should notify the user of the failure.
	Error error
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
	// Keep requests explicit delivery to the default mailbox. The RFC 5228
	// §2.10.2 implicit keep does not call this method; it is reported through
	// Outcome.ImplicitKeep so the host can distinguish it from an explicit keep.
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
//
// It applies the RFC 5228 §2.10.2 implicit-keep model: unless the script cancels
// it (with keep, fileinto, redirect, or discard) the message is delivered to the
// default mailbox, reported via [Outcome.ImplicitKeep]. Per §2.10.6, a runtime
// error during evaluation fails safe to that implicit keep rather than losing the
// message.
func (s *Script) Evaluate(msg *Message, exec Executor) (out Outcome) {
	// A nil script and a nil message both leave nothing to decide. A nil message
	// in particular has no headers, body, or envelope to test against, and every
	// test dereferences msg, so evaluating one would panic. Either way, take the
	// implicit keep: deliver to the default mailbox, honouring whatever the
	// Executor recorded (here, nothing).
	if s == nil || msg == nil {
		return Outcome{Disposition: Continue, ImplicitKeep: true}
	}
	st := &evalState{
		msg:          msg,
		exec:         exec,
		implicitKeep: true,
		filedInto:    map[fileintoDest]struct{}{},
		redirected:   map[redirectDest]struct{}{},
	}
	// RFC 5228 §2.10.6: an execution error must not lose the message. If any
	// action panics — typically a host Executor callback failing — recover and
	// fall back to the implicit keep, reporting the error so the host can notify
	// the user.
	defer func() {
		if r := recover(); r != nil {
			out = Outcome{Disposition: Continue, ImplicitKeep: true, Error: errFromRecover(r)}
		}
	}()
	st.runBlock(s.commands)
	return st.finalize()
}

// errFromRecover normalises a recovered panic value into an error.
func errFromRecover(r any) error {
	if err, ok := r.(error); ok {
		return err
	}
	return fmt.Errorf("sieve: evaluation aborted by runtime error: %v", r)
}

// evalState carries the immutable inputs while walking a script's commands, the
// implicit-keep bookkeeping, and the set of delivery destinations already
// dispatched (for RFC 5228 §2.10.3 de-duplication).
type evalState struct {
	msg  *Message
	exec Executor

	// implicit-keep model (RFC 5228 §2.10.2). The final delivery decision is
	// resolved by finalize after the whole script has run (or after stop),
	// rather than inline, so that discard does not terminate the script and a
	// later delivering action can still take effect.
	//
	//   implicitKeep — true until a keep, fileinto, redirect, or discard cancels
	//                  it. When still true at the end, the message is delivered to
	//                  the default mailbox (Outcome.ImplicitKeep).
	//   delivered    — a keep, fileinto, or redirect ran, so the message has an
	//                  explicit destination. Delivering actions win over discard.
	//   discarded    — discard ran; it cancels the implicit keep but does not
	//                  stop the script. If no delivering action also ran, the
	//                  message is dropped.
	//   rejected     — reject ran; a terminal refusal that halts the script.
	implicitKeep bool
	delivered    bool
	discarded    bool
	rejected     bool
	rejectReason string

	// Delivery de-duplication (RFC 5228 §2.10.3): "the same message MUST NOT be
	// delivered to the same destination more than once", even when several
	// actions name it. These record the destinations already dispatched so a
	// repeated keep / fileinto / redirect collapses to a single Executor call.
	// The first occurrence executes (preserving script order and interleaving
	// with other actions); later duplicates are suppressed. A :copy action is a
	// distinct destination from a consuming one, so :copy is part of each key.
	kept       bool
	filedInto  map[fileintoDest]struct{}
	redirected map[redirectDest]struct{}
}

// fileintoDest keys a fileinto delivery for de-duplication: same folder and
// same :copy disposition means the same destination.
type fileintoDest struct {
	folder string
	copy   bool
}

// redirectDest keys a redirect delivery for de-duplication: same address and
// same :copy disposition means the same destination.
type redirectDest struct {
	addr string
	copy bool
}

// runBlock evaluates a sequence of commands, recording their effects on the
// evaluation state. The bool result reports whether evaluation should halt (a
// stop, or a terminal reject); the resolved delivery is computed by finalize
// once the script (or the halting command) is reached.
func (st *evalState) runBlock(cmds []sieveCmd) bool {
	for _, c := range cmds {
		if st.runCmd(c) {
			return true
		}
	}
	return false
}

// runCmd applies a single command, mutating the evaluation state. It returns
// true to halt all further processing (stop or reject).
func (st *evalState) runCmd(c sieveCmd) (halt bool) {
	switch cmd := c.(type) {
	case *ifCmd:
		for _, br := range cmd.branches {
			if br.test == nil || evalTest(br.test, st.msg) {
				return st.runBlock(br.block)
			}
		}

	case *stopCmd:
		// Halt further Sieve processing. Whatever the state is at this point —
		// an implicit keep still owed, an explicit delivery already recorded, or
		// a discard with nothing else — is what finalize will resolve.
		return true

	case *keepCmd:
		st.deliverKeep()

	case *discardCmd:
		// RFC 5228 §4.4: discard cancels the implicit keep and is compatible with
		// every other action. It does NOT stop the script; subsequent actions
		// still run, and if a delivering action also runs it overrides the discard
		// ("fileinto"+"discard" is equivalent to "fileinto"). Only when no
		// delivering action runs does discard drop the message.
		st.implicitKeep = false
		st.discarded = true

	case *rejectCmd:
		// A terminal refusal: it cancels the implicit keep and halts the script.
		st.implicitKeep = false
		st.rejected = true
		st.rejectReason = cmd.reason
		return true

	case *fileintoCmd:
		st.deliverFileInto(cmd)

	case *redirectCmd:
		st.deliverRedirect(cmd)

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

	return false
}

// deliverKeep records an explicit keep: it cancels the implicit keep and
// delivers to the default mailbox. RFC 5228 §2.10.3: multiple keeps collapse to
// a single delivery.
func (st *evalState) deliverKeep() {
	st.implicitKeep = false
	st.delivered = true
	if !st.kept {
		st.kept = true
		st.exec.Keep()
	}
}

// deliverFileInto records a fileinto: it cancels the implicit keep and files the
// message. RFC 5228 §2.10.3: filing into the same mailbox twice is one delivery.
func (st *evalState) deliverFileInto(cmd *fileintoCmd) {
	st.implicitKeep = false
	st.delivered = true
	dest := fileintoDest{folder: cmd.folder, copy: cmd.copy}
	if _, done := st.filedInto[dest]; !done {
		st.filedInto[dest] = struct{}{}
		st.exec.FileInto(cmd.folder, cmd.create)
	}
}

// deliverRedirect records a redirect: it cancels the implicit keep and forwards
// the message. RFC 5228 §2.10.3: redirecting to the same address twice is one
// delivery.
func (st *evalState) deliverRedirect(cmd *redirectCmd) {
	st.implicitKeep = false
	st.delivered = true
	dest := redirectDest{addr: cmd.addr, copy: cmd.copy}
	if _, done := st.redirected[dest]; !done {
		st.redirected[dest] = struct{}{}
		st.exec.Redirect(cmd.addr)
	}
}

// finalize resolves the accumulated state into the terminal Outcome, applied
// once the whole script has run or a stop/reject halted it.
func (st *evalState) finalize() Outcome {
	switch {
	case st.rejected:
		return Outcome{Disposition: Reject, RejectReason: st.rejectReason}
	case st.implicitKeep:
		// Nothing cancelled the implicit keep: deliver to the default mailbox.
		return Outcome{Disposition: Continue, ImplicitKeep: true}
	case st.delivered:
		// A keep, fileinto, or redirect ran: deliver per the recorded actions. If
		// a discard also ran it is overridden (RFC 5228 §4.4).
		return Outcome{Disposition: Continue}
	default:
		// The implicit keep was cancelled only by discard, with no delivering
		// action: silently drop the message.
		return Outcome{Disposition: Discard}
	}
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

// messageSize returns the octet count the size test (RFC 5228 §5.9) compares
// against: the number of octets in the entire message — the header block, the
// blank line separating headers from body, and the body — as it appears on the
// wire. When the host supplied the exact wire size in RawSize that value is used
// verbatim; otherwise the size is reconstructed from the header, body, and
// attachment fields (an approximation, since the reconstructed headers need not
// be byte-identical to the original transfer encoding).
func messageSize(msg *Message) int64 {
	if msg.RawSize > 0 {
		return msg.RawSize
	}
	size := headerSize(msg.Headers)
	size += 2 // CRLF blank line separating the header block from the body
	size += bodySize(msg.Body)
	for _, a := range msg.Attachments {
		size += a.Size
	}
	return size
}

// headerSize approximates the octet size of the serialized header block,
// rendering each present header as "Name: value\r\n". It is only used when the
// host did not supply RawSize.
func headerSize(h Headers) int64 {
	var size int64
	add := func(name, value string) {
		if value == "" {
			return
		}
		size += int64(len(name) + len(": ") + len(value) + len("\r\n"))
	}
	add("Subject", h.Subject)
	add("From", strings.Join(formatAddresses(h.From), ", "))
	add("To", strings.Join(formatAddresses(h.To), ", "))
	add("Cc", strings.Join(formatAddresses(h.Cc), ", "))
	add("Bcc", strings.Join(formatAddresses(h.Bcc), ", "))
	add("Message-ID", h.MessageID)
	add("In-Reply-To", h.InReplyTo)
	add("Date", h.Date)
	add("References", strings.Join(h.References, " "))
	for k, vals := range h.Raw {
		for _, v := range vals {
			add(k, v)
		}
	}
	return size
}

// Body traversal is bounded so a message supplied by the host cannot turn the
// recursive walk of its MIME structure into a denial of service (issue #23).
const (
	// maxBodyDepth caps how many levels deep the body test and the size
	// reconstruction descend into nested multipart structure. Real messages
	// nest only a handful of levels; a subtree deeper than this is treated as
	// having no further parts rather than recursed into, so a pathologically
	// deep Parts chain cannot exhaust the goroutine stack.
	maxBodyDepth = 100
	// maxBodyParts caps the total number of MIME parts a single body traversal
	// visits, so a part-heavy message cannot burn unbounded CPU. Once the budget
	// is spent the remaining parts are treated as absent.
	maxBodyParts = 10000
)

func bodySize(b Body) int64 {
	budget := maxBodyParts
	return bodySizeBounded(b, maxBodyDepth, &budget)
}

// bodySizeBounded sums the content octets of a body and its parts, descending at
// most depthLeft further levels and visiting at most *budget more parts across
// the whole walk. Beyond either bound a subtree is treated as having no further
// content, so reconstructing an approximate wire size for a pathologically deep
// or part-heavy message cannot exhaust the stack or burn unbounded CPU.
func bodySizeBounded(b Body, depthLeft int, budget *int) int64 {
	size := int64(len(b.Content))
	if depthLeft <= 0 {
		return size
	}
	for _, p := range b.Parts {
		if *budget <= 0 {
			break
		}
		*budget--
		size += bodySizeBounded(p, depthLeft-1, budget)
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
	// RFC 5228 §5.7: the header test compares the value of the named field
	// with leading and trailing whitespace ignored, after RFC 5322 §2.2.3
	// unfolding. Normalise every value so a padded or folded header still
	// matches :is / :contains against the logical value.
	for i, v := range out {
		out[i] = unfoldAndTrim(v)
	}
	return out
}

// unfoldAndTrim applies RFC 5322 §2.2.3 unfolding to a header value — each line
// break (CRLF, or a bare CR/LF) that introduces folding whitespace is removed
// along with that whitespace and replaced by a single space — and then strips
// leading and trailing whitespace, as RFC 5228 §5.7 requires the header test to
// ignore. Values without a line break take a fast path that only trims.
func unfoldAndTrim(v string) string {
	if !strings.ContainsAny(v, "\r\n") {
		return strings.TrimSpace(v)
	}
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); i++ {
		if c := v[i]; c == '\r' || c == '\n' {
			// Collapse the line break and any adjoining folding whitespace
			// (further CR/LF, SP, or HTAB) into a single space.
			for i+1 < len(v) {
				if n := v[i+1]; n == '\r' || n == '\n' || n == ' ' || n == '\t' {
					i++
					continue
				}
				break
			}
			b.WriteByte(' ')
			continue
		}
		b.WriteByte(v[i])
	}
	return strings.TrimSpace(b.String())
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

// addressList returns the bare addr-specs of a header (no display name). For
// headers outside the structured set, the raw value is parsed as an RFC 5322
// address list so the address test sees real addr-specs — not display names,
// comments, or group syntax — per RFC 5228 §5.1. Values that do not parse as an
// address are dropped so they cannot match :localpart/:domain (§2.7.4).
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
		if !strings.EqualFold(k, lower) {
			continue
		}
		for _, v := range vals {
			out = append(out, parseAddrSpecs(v)...)
		}
	}
	return out
}

// parseAddrSpecs parses a raw header value as an RFC 5322 address list and
// returns the addr-spec of each address. A value that cannot be parsed as an
// address list yields no addr-specs.
func parseAddrSpecs(raw string) []string {
	addrs, err := mail.ParseAddressList(raw)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if a.Address != "" {
			out = append(out, a.Address)
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
		// A null reverse-path (MAIL FROM:<>) is the empty string, not an
		// absent value: RFC 5228 §5.4 requires `:is "from" ""` to match it.
		// A genuinely absent sender (no MAIL FROM) yields no value.
		if msg.Envelope.FromNull {
			return []string{""}
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
		return strings.Contains(asciiCasemapFold(value), asciiCasemapFold(key))
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
		return strings.Contains(asciiCasemapFold(value), asciiCasemapFold(key))
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
		return asciiCasemapFold(value) == asciiCasemapFold(key)
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
	return asciiCasemapFold(s)
}

// asciiCasemapFold applies the i;ascii-casemap comparator's case fold
// (RFC 4790 §9.2, RFC 5228 §2.7.3): ONLY the US-ASCII letters A–Z (0x41–0x5A)
// fold to a–z. Every other octet — all non-letter ASCII and every byte ≥ 0x80,
// including the interior bytes of multi-byte UTF-8 sequences — is left
// byte-exact, so non-ASCII characters are compared unchanged and cannot be
// folded onto an ASCII look-alike (e.g. ſ U+017F, Kelvin sign K U+212A). This
// deliberately avoids Go's Unicode-aware strings.ToLower/EqualFold, which fold
// beyond US-ASCII and would let a filter be evaded or over-match.
func asciiCasemapFold(s string) string {
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
// and returns the first match's content. The search is bounded in both nesting
// depth and total parts visited so a pathologically deep or part-heavy MIME
// structure cannot turn it into a denial of service (issue #23).
func findPartContent(parts []Body, contentType string) string {
	budget := maxBodyParts
	return findPartContentBounded(parts, contentType, maxBodyDepth, &budget)
}

// findPartContentBounded searches parts for the first matching content type,
// descending at most depthLeft further levels and visiting at most *budget more
// parts across the whole walk. Beyond either bound the deeper or remaining parts
// are treated as absent rather than recursed into.
func findPartContentBounded(parts []Body, contentType string, depthLeft int, budget *int) string {
	if depthLeft <= 0 {
		return ""
	}
	for _, p := range parts {
		if *budget <= 0 {
			return ""
		}
		*budget--
		if strings.HasPrefix(strings.ToLower(p.ContentType), contentType) && p.Content != "" {
			return p.Content
		}
		if found := findPartContentBounded(p.Parts, contentType, depthLeft-1, budget); found != "" {
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
