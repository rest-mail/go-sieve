package sieve

import (
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"github.com/rest-mail/go-sieve/pkg/sieve/internal/match"
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
	// NoBody marks a message that is a header block with no empty line after
	// it, so it has no body at all. An empty Body cannot say this: a message
	// whose body is the empty string still has one. Every body test of a
	// message without a body is false, even one for the empty string (RFC 5173
	// §4).
	NoBody bool
	// RawBody is the undecoded body exactly as it appears on the wire:
	// everything after the empty line that ends the header, the empty line
	// itself excluded (RFC 5173 §4). Transfer encodings, multipart boundaries,
	// prologues, epilogues and the MIME headers of nested parts all stay in
	// it. The body :raw transform matches it as one string (RFC 5173 §5.1); the
	// :text and :content transforms never read it.
	RawBody string
	// RawBodySet reports that the host supplied RawBody, so an empty RawBody
	// is a body that is really empty. When it is false the evaluator has no
	// undecoded body, and evaluating body :raw is a runtime error,
	// [ErrRawBodyUnavailable], rather than a match against decoded text, which
	// RFC 5173 §5.1 forbids. NoBody still makes every body test false first.
	RawBodySet bool
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
	// To supplies the final recipient identities. When evaluating a user's
	// vacation script, include only identities belonging to that user: these
	// are matched against the original recipient headers (RFC 5230 §4.5).
	To []string
}

// Body is a (possibly multipart) message body used by the body test and size.
type Body struct {
	ContentType string
	// Content is the part's text, decoded and in UTF-8: the host removes the
	// content transfer encoding and converts the charset (RFC 5173 §5.2). For
	// a multipart or message/rfc822 part, :content matches whatever the host
	// puts here, such as the prologue or the nested header.
	Content string
	Parts   []Body
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
	// ExtendedReject is true only when the executed refusal was ereject, rather
	// than reject (RFC 5429). Both use Disposition == Reject and RejectReason.
	// The host must preserve the distinction: ereject prefers protocol-level
	// refusal with a DSN fallback (§2.1), while reject uses the policy in §2.2.
	ExtendedReject bool
	// ImplicitKeep reports that the RFC 5228 §2.10.2 implicit keep is in effect:
	// the script performed no keep, discard, or fileinto or redirect without
	// :copy, so the host must deliver the message to the default mailbox in
	// addition to honouring any actions the Executor recorded. A fileinto or
	// redirect tagged :copy leaves it in effect (RFC 3894 §3), but does not
	// restore it once another action has cancelled it. It is only ever set
	// alongside Disposition == Continue. It is also set on the §2.10.6 fail-safe
	// keep (see Error). When a keep, discard, or non-:copy fileinto or redirect
	// ran, ImplicitKeep is false.
	ImplicitKeep bool
	// ImplicitKeepFlags is the RFC 5232 internal flag variable when the
	// evaluation ended (or, on a runtime error, when it stopped): the IMAP
	// flags the implicit keep stores the message with (RFC 5232 §3). It is set
	// only when ImplicitKeep is true; an empty list means no flags.
	ImplicitKeepFlags []string
	// Error is non-nil when evaluation was aborted by a runtime error — for
	// example an Executor callback panicked. Per RFC 5228 §2.10.6 the message is
	// then kept rather than lost (ImplicitKeep is true and Disposition is
	// Continue) and the host should notify the user of the failure.
	Error error
}

// Vacation carries a matched vacation action (RFC 5230). The evaluator checks
// personal addressing and suppresses automated/list mail. The Executor handles
// persistent response tracking and generates and sends the response message.
// It must use the explicit Handle when HandleSet, otherwise the original
// Subject, From, MIME and Body arguments, to identify the response (§4.2).
// Track separately for each recipient user and ReplyTo, honoring Interval.
// Generated defaults (notably Subject) must not change the tracking identity.
type Vacation struct {
	// Interval is the minimum period between replies to the same sender for
	// this response: :days is clamped to 1..365, default 7; :seconds retains
	// its sub-day precision, including zero, with a maximum of 2147483647
	// seconds (RFC 6131). It requires vacation-seconds, which implies vacation.
	Interval time.Duration
	Subject  string
	// SubjectSet distinguishes an explicit empty subject from an omitted one.
	// If omitted the host generates "Auto: " + original subject, or a fixed
	// fallback if the original message has no Subject (§5.3).
	SubjectSet bool
	Body       string
	ReplyTo    string
	// From is the optional validated mailbox-list; empty means use the owner.
	From string
	// MIME says Body includes MIME headers as well as content; otherwise Body
	// is UTF-8 text. The host validates and encodes the outgoing MIME entity.
	MIME   bool
	Handle string
	// HandleSet distinguishes an explicit empty tracking handle from no handle.
	HandleSet bool
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
	// FileInto delivers into the named folder. When create is true, the host
	// must create a missing folder before delivery and leave an existing one
	// alone (RFC 5490 §3.2). Creation failure is a runtime error: this callback
	// can panic with an error, which Evaluate reports through Outcome.Error
	// with the fail-safe implicit keep. Merely recording a failure in the host
	// does not notify the evaluator; the callback has no error return.
	FileInto(folder string, create bool)
	// Redirect forwards the message to addr. copy reports that the action was
	// tagged :copy (RFC 3894): it forwards a copy without cancelling the implicit
	// keep. Whether the message is also kept is decided by Outcome.ImplicitKeep
	// and any explicit Keep, never by copy alone. A host that can decline a
	// redirect implements [RedirectReporter] instead.
	Redirect(addr string, copy bool)
	// Flag applies an imap4flags operation ("setflag", "addflag", "removeflag").
	// flags is already cleaned up as RFC 5232 §2 requires: one valid, settable
	// IMAP flag per element, each given once. It may be empty.
	Flag(op string, flags []string)
	// Vacation records a vacation auto-reply request.
	Vacation(v Vacation)
	// Notify records a notification request.
	Notify(method, message string)
}

// FlagDeliverer is an optional extension of [Executor] for hosts that store
// RFC 5232 flags with each delivery. When the Executor passed to
// [Script.Evaluate] also implements it, every explicit keep and fileinto is
// delivered through these methods instead of Keep and FileInto, with flags set
// to the effective list: the :flags argument when the action has one,
// otherwise the internal flag variable at the time the action ran (RFC 5232
// §3, §5). The message is stored with only these flags. The implicit keep's
// flags are reported in [Outcome.ImplicitKeepFlags].
//
// RFC 5232 §3 requires the last flag list to win when repeated deliveries to
// one destination are merged (RFC 5228 §2.10.3). To honour that, these calls
// are made once the script has finished, one per destination, in the order
// each destination was first named, after the other callbacks. The other
// arguments of a merged delivery are those of its first occurrence. A reject
// makes none of these calls. Flag is still called for each setflag, addflag
// and removeflag.
type FlagDeliverer interface {
	// KeepWithFlags delivers to the default mailbox with flags.
	KeepWithFlags(flags []string)
	// FileIntoWithFlags delivers into folder with flags; create is as for
	// [Executor.FileInto].
	FileIntoWithFlags(folder string, create bool, flags []string)
}

// ErrRedirectSkipped is returned by [RedirectReporter.TryRedirect] when host
// policy silently declined a redirect, for example because the script reached
// the host's redirect limit (RFC 5228 §4.2, §10).
var ErrRedirectSkipped = errors.New("sieve: redirect skipped by host policy")

// RedirectReporter is an optional extension of [Executor] for hosts that
// can decline a redirect. When the Executor passed to [Script.Evaluate] also
// implements it, each redirect calls TryRedirect instead of Redirect, and the
// result decides what the redirect did:
//
//   - nil: the message was forwarded. A redirect without :copy cancels the
//     implicit keep.
//   - an error matching ErrRedirectSkipped (errors.Is): policy silently
//     ignored the redirect. It does not cancel the implicit keep (RFC 5228
//     §4.2) and does not count as a delivery, so a discard is not overridden
//     by it. A repeated redirect to the same destination is skipped again
//     without a second call.
//   - any other error: a runtime failure. Evaluation stops and fails safe to
//     the implicit keep, reported through [Outcome.Error] (RFC 5228 §2.10.6).
type RedirectReporter interface {
	TryRedirect(addr string, copy bool) error
}

// ErrRawBodyUnavailable is the [Outcome.Error] of an evaluation that reached
// a body :raw test for a [Message] without RawBodySet. RFC 5173 §5.1 requires
// :raw to match the undecoded body, so the evaluator does not substitute the
// decoded text; the runtime error fails safe to the implicit keep (RFC 5228
// §2.10.6).
var ErrRawBodyUnavailable = errors.New("sieve: body :raw needs the undecoded body (Message.RawBody) and the host did not supply it")

// ErrMailboxQueryUnavailable is the [Outcome.Error] of an evaluation that
// reached a mailboxexists test while the Executor does not implement
// [MailboxChecker]. Without the query the evaluator cannot tell whether a
// mailbox exists, so it neither guesses true nor false: the runtime error
// fails safe to the implicit keep (RFC 5228 §2.10.6).
var ErrMailboxQueryUnavailable = errors.New("sieve: mailboxexists needs a mailstore query (MailboxChecker) and the host did not supply one")

// MailboxChecker is an optional extension of [Executor] for hosts that can
// answer the RFC 5490 §3.1 mailboxexists test. When the Executor passed to
// [Script.Evaluate] also implements it, mailboxexists calls MailboxExists once
// for each named mailbox, in order, until one is not deliverable; the test is
// true only when every named mailbox is. Without it, mailboxexists is a
// runtime error ([ErrMailboxQueryUnavailable]).
//
// The query is scoped to the user the script runs for, which the host knows
// from the Executor it built for this evaluation. MailboxExists reports:
//
//   - true, nil: the mailbox exists and that user may deliver into it. For an
//     IMAP mailstore that is a READ-WRITE mailbox when the server has no ACL
//     support, or the user's 'p' or 'i' right (RFC 4314 §5.2) when it has.
//     It promises nothing about a later fileinto, which may still fail, for
//     example over quota: delivery and quota stay with the host.
//   - false, nil: the mailbox does not exist or the user may not deliver into
//     it. The test is false and the script goes on.
//   - any error: the mailstore could not answer, for example a temporary
//     failure. Evaluation stops and fails safe to the implicit keep, reported
//     through [Outcome.Error] (RFC 5228 §2.10.6), so a lookup failure is never
//     read as a missing mailbox.
type MailboxChecker interface {
	MailboxExists(name string) (bool, error)
}

// Validate reports whether a Sieve script is syntactically valid and its
// actions are compatible (see [Parse]). It accepts
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
// it (with keep, discard, or a fileinto or redirect without :copy) the message is
// delivered to the default mailbox, reported via [Outcome.ImplicitKeep]. Per
// §2.10.6, a runtime error during evaluation fails safe to that implicit keep
// rather than losing the message.
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
		filedInto:    map[fileintoDest]int{},
		redirected:   map[redirectDest]bool{},
		flags:        []string{},
	}
	st.flagDeliverer, _ = exec.(FlagDeliverer)
	st.redirectReporter, _ = exec.(RedirectReporter)
	st.mailboxChecker, _ = exec.(MailboxChecker)
	// RFC 5228 §2.10.6: an execution error must not lose the message. If any
	// action panics — typically a host Executor callback failing — recover and
	// fall back to the implicit keep, reporting the error so the host can notify
	// the user.
	defer func() {
		if r := recover(); r != nil {
			out = Outcome{Disposition: Continue, ImplicitKeep: true, ImplicitKeepFlags: st.flagList(), Error: errFromRecover(r)}
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
	//   implicitKeep — true until a keep, discard, or a fileinto or redirect
	//                  without :copy cancels it (RFC 3894 §3: :copy does not).
	//                  Nothing sets it back. When still true at the end, the
	//                  message is delivered to the default mailbox
	//                  (Outcome.ImplicitKeep).
	//   delivered    — a keep, fileinto, or redirect ran, so the message has an
	//                  explicit destination. Delivering actions win over discard.
	//   discarded    — discard ran; it cancels the implicit keep but does not
	//                  stop the script. If no delivering action also ran, the
	//                  message is dropped.
	//   rejected     — reject ran; a terminal refusal that halts the script.
	implicitKeep   bool
	delivered      bool
	discarded      bool
	rejected       bool
	rejectReason   string
	extendedReject bool

	// Delivery de-duplication (RFC 5228 §2.10.3): "the same message MUST NOT be
	// delivered to the same destination more than once", even when several
	// actions name it. These record the destinations already dispatched so a
	// repeated keep / fileinto / redirect collapses to a single Executor call.
	// The first occurrence executes (preserving script order and interleaving
	// with other actions); later duplicates are suppressed. A :copy action is a
	// distinct destination from a consuming one, so :copy is part of each key.
	// With a FlagDeliverer the keep and fileinto calls wait for finalize instead
	// (see pending), so the last flag list can win. redirected maps each
	// destination to whether host policy skipped it (RedirectReporter).
	kept       bool
	filedInto  map[fileintoDest]int // index into pending, or -1 once dispatched
	redirected map[redirectDest]bool

	// flags is the RFC 5232 internal variable: the flag set setflag, addflag
	// and removeflag change, hasflag tests, and deliveries without :flags use.
	flags []string

	// Optional Executor extensions, nil when the host does not implement them.
	flagDeliverer    FlagDeliverer
	redirectReporter RedirectReporter
	mailboxChecker   MailboxChecker
	// pending holds the keep and fileinto deliveries a FlagDeliverer receives
	// at finalize, one per destination, in first-occurrence order.
	pending []pendingDelivery

	// Even a suppressed vacation counts as an execution (RFC 5230 §4.7).
	vacationExecuted bool
}

// pendingDelivery is a keep (keep true) or fileinto waiting for finalize, with
// the flag list of its last occurrence.
type pendingDelivery struct {
	keep   bool
	folder string
	create bool
	flags  []string
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
			if br.test == nil || st.evalTest(br.test) {
				return st.runBlock(br.block)
			}
		}

	case *stopCmd:
		// Halt further Sieve processing. Whatever the state is at this point —
		// an implicit keep still owed, an explicit delivery already recorded, or
		// a discard with nothing else — is what finalize will resolve.
		return true

	case *keepCmd:
		st.deliverKeep(cmd)

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
		st.extendedReject = cmd.extended
		return true

	case *fileintoCmd:
		st.deliverFileInto(cmd)

	case *redirectCmd:
		st.deliverRedirect(cmd)

	case *flagCmd:
		st.applyFlag(cmd.op, cmd.flags)
		st.exec.Flag(cmd.op, cmd.flags)

	case *vacationCmd:
		if st.vacationExecuted {
			panic(fmt.Errorf("sieve: vacation may execute only once per script"))
		}
		st.vacationExecuted = true
		if replyTo, ok := st.vacationReplyTo(); ok && st.vacationAllowed(cmd, replyTo) {
			st.exec.Vacation(Vacation{
				Interval:   cmd.interval,
				Subject:    cmd.subject,
				SubjectSet: cmd.subjectSet,
				Body:       cmd.body,
				ReplyTo:    replyTo,
				From:       cmd.from,
				MIME:       cmd.mime,
				Handle:     cmd.handle,
				HandleSet:  cmd.handleSet,
			})
		}

	case *notifyCmd:
		st.exec.Notify(cmd.method, cmd.message)
	}

	return false
}

// applyFlag applies setflag, addflag or removeflag to the internal variable
// (RFC 5232 §3.1–3.3). flags is already normalised; flag names compare
// case-insensitively, so the set never holds the same flag twice.
func (st *evalState) applyFlag(op string, flags []string) {
	switch op {
	case "setflag":
		st.flags = append([]string{}, flags...)
	case "addflag":
		for _, f := range flags {
			if !containsFold(st.flags, f) {
				st.flags = append(st.flags, f)
			}
		}
	case "removeflag":
		kept := []string{}
		for _, f := range st.flags {
			if !containsFold(flags, f) {
				kept = append(kept, f)
			}
		}
		st.flags = kept
	}
}

// containsFold reports whether list holds f, compared ASCII case-insensitively
// as IMAP flag names are.
func containsFold(list []string, f string) bool {
	for _, g := range list {
		if strings.EqualFold(g, f) {
			return true
		}
	}
	return false
}

// flagList returns a copy of the internal variable.
func (st *evalState) flagList() []string { return append([]string{}, st.flags...) }

// deliveryFlags is the effective flag list of a keep or fileinto: its :flags
// argument when given, otherwise the internal variable (RFC 5232 §5).
func (st *evalState) deliveryFlags(flags []string, set bool) []string {
	if set {
		return append([]string{}, flags...)
	}
	return st.flagList()
}

// deliverKeep records an explicit keep: it cancels the implicit keep and
// delivers to the default mailbox. RFC 5228 §2.10.3: multiple keeps collapse to
// a single delivery; with a FlagDeliverer the last one's flags win.
func (st *evalState) deliverKeep(cmd *keepCmd) {
	st.implicitKeep = false
	st.delivered = true
	if st.flagDeliverer != nil {
		flags := st.deliveryFlags(cmd.flags, cmd.flagsSet)
		for i := range st.pending {
			if st.pending[i].keep {
				st.pending[i].flags = flags
				return
			}
		}
		st.pending = append(st.pending, pendingDelivery{keep: true, flags: flags})
		return
	}
	if !st.kept {
		st.kept = true
		st.exec.Keep()
	}
}

// deliverFileInto records a fileinto: it files the message and, unless tagged
// :copy (RFC 3894 §3), cancels the implicit keep. RFC 5228 §2.10.3: filing into
// the same mailbox twice is one delivery.
func (st *evalState) deliverFileInto(cmd *fileintoCmd) {
	if !cmd.copy {
		st.implicitKeep = false
	}
	st.delivered = true
	dest := fileintoDest{folder: cmd.folder, copy: cmd.copy}
	i, done := st.filedInto[dest]
	if st.flagDeliverer != nil {
		flags := st.deliveryFlags(cmd.flags, cmd.flagsSet)
		if done {
			st.pending[i].flags = flags
			return
		}
		st.filedInto[dest] = len(st.pending)
		st.pending = append(st.pending, pendingDelivery{folder: cmd.folder, create: cmd.create, flags: flags})
		return
	}
	if !done {
		st.filedInto[dest] = -1
		st.exec.FileInto(cmd.folder, cmd.create)
	}
}

// deliverRedirect records a redirect: it forwards the message and, unless tagged
// :copy (RFC 3894 §3), cancels the implicit keep. RFC 5228 §2.10.3: redirecting
// to the same address twice is one delivery. A redirect host policy skipped
// (RedirectReporter) has no effect at all: RFC 5228 §4.2 says it must not
// cancel the implicit keep, and nothing was delivered.
func (st *evalState) deliverRedirect(cmd *redirectCmd) {
	dest := redirectDest{addr: cmd.addr, copy: cmd.copy}
	skipped, done := st.redirected[dest]
	if !done {
		skipped = st.redirect(cmd.addr, cmd.copy)
		st.redirected[dest] = skipped
	}
	if skipped {
		return
	}
	if !cmd.copy {
		st.implicitKeep = false
	}
	st.delivered = true
}

// redirect hands one redirect to the host and reports whether policy skipped
// it. Any other error from TryRedirect is a runtime failure: it panics into the
// fail-safe keep of Evaluate (RFC 5228 §2.10.6).
func (st *evalState) redirect(addr string, copy bool) (skipped bool) {
	if st.redirectReporter == nil {
		st.exec.Redirect(addr, copy)
		return false
	}
	err := st.redirectReporter.TryRedirect(addr, copy)
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRedirectSkipped) {
		return true
	}
	panic(err)
}

// finalize resolves the accumulated state into the terminal Outcome, applied
// once the whole script has run or a stop/reject halted it.
func (st *evalState) finalize() Outcome {
	if !st.rejected {
		for _, d := range st.pending {
			if d.keep {
				st.flagDeliverer.KeepWithFlags(d.flags)
			} else {
				st.flagDeliverer.FileIntoWithFlags(d.folder, d.create, d.flags)
			}
		}
	}
	switch {
	case st.rejected:
		return Outcome{Disposition: Reject, RejectReason: st.rejectReason, ExtendedReject: st.extendedReject}
	case st.implicitKeep:
		// Nothing cancelled the implicit keep: deliver to the default mailbox, in
		// addition to any :copy fileinto or redirect the Executor recorded, with
		// the internal flag variable (RFC 5232 §3).
		return Outcome{Disposition: Continue, ImplicitKeep: true, ImplicitKeepFlags: st.flagList()}
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

// vacationReplyTo uses only the SMTP reverse-path (§4), never From or Reply-To.
// A null or unavailable reverse-path cannot receive a reply (RFC 3834).
func (st *evalState) vacationReplyTo() (addr string, ok bool) {
	if st.msg.Envelope.FromNull || st.msg.Envelope.From == "" {
		return "", false
	}
	return st.msg.Envelope.From, true
}

// ── Test evaluation ──────────────────────────────────────────────────

// evalTest evaluates t against the message. The hasflag test reads the RFC
// 5232 internal variable and mailboxexists queries the host.
func (st *evalState) evalTest(t sieveTest) bool {
	msg := st.msg
	switch tt := t.(type) {
	case *allofTest:
		for _, sub := range tt.tests {
			if !st.evalTest(sub) {
				return false
			}
		}
		return true
	case *anyofTest:
		for _, sub := range tt.tests {
			if st.evalTest(sub) {
				return true
			}
		}
		return false
	case *notTest:
		return !st.evalTest(tt.inner)
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
		if tt.matchType == ":count" {
			return countMatches(headerFieldCount(msg, tt.headers), tt.relation, tt.comparator, tt.keys)
		}
		values := gatherHeaderValues(msg, tt.headers)
		return matchValues(values, tt.matchType, tt.relation, tt.comparator, tt.keys)
	case *addressTest:
		if tt.matchType == ":count" {
			n := 0
			for _, h := range tt.headers {
				n += len(addressList(msg, h))
			}
			return countMatches(n, tt.relation, tt.comparator, tt.keys)
		}
		values := gatherAddressValues(msg, tt.headers, tt.addressPart, tt.subaddress)
		return matchValues(values, tt.matchType, tt.relation, tt.comparator, tt.keys)
	case *envelopeTest:
		if tt.matchType == ":count" {
			return countMatches(envelopeCount(msg, tt.parts), tt.relation, tt.comparator, tt.keys)
		}
		values := gatherEnvelopeValues(msg, tt.parts, tt.addressPart, tt.subaddress)
		return matchValues(values, tt.matchType, tt.relation, tt.comparator, tt.keys)
	case *bodyTest:
		if msg.NoBody {
			return false
		}
		return matchValues(bodyValues(msg, tt), tt.matchType, tt.relation, tt.comparator, tt.keys)
	case *hasflagTest:
		return hasflag(st.flags, tt)
	case *mailboxexistsTest:
		return st.mailboxesExist(tt.mailboxes)
	}
	return false
}

// mailboxesExist implements the RFC 5490 §3.1 test: true only when every
// named mailbox exists and accepts delivery by the script's user. A query
// error, or a host without the query, is a runtime error that panics into the
// fail-safe keep of Evaluate (RFC 5228 §2.10.6).
func (st *evalState) mailboxesExist(names []string) bool {
	if st.mailboxChecker == nil {
		panic(ErrMailboxQueryUnavailable)
	}
	for _, name := range names {
		ok, err := st.mailboxChecker.MailboxExists(name)
		if err != nil {
			panic(err)
		}
		if !ok {
			return false
		}
	}
	return true
}

// hasflag implements the RFC 5232 §4 test on the internal variable. Each flag
// of the variable is a value and each space-separated name in a key is a key,
// so "b A" is the same key list as ["b", "A"]. The test is true if any flag
// matches any key. :count compares the number of distinct flags, which the
// variable holds once each.
func hasflag(flags []string, tt *hasflagTest) bool {
	if tt.matchType == ":count" {
		return countMatches(len(flags), tt.relation, tt.comparator, tt.keys)
	}
	var keys []string
	for _, k := range tt.keys {
		keys = append(keys, strings.Fields(k)...)
	}
	return matchValues(flags, tt.matchType, tt.relation, tt.comparator, keys)
}

// matchValues applies a test's match type to the values taken from the
// message: the relational :value (RFC 5231 §4.1) or one of the others.
func matchValues(values []string, matchType, relation, comparator string, keys []string) bool {
	if matchType == ":value" {
		return match.AnyValue(values, relation, comparator, keys)
	}
	return match.Any(values, matchType, comparator, keys)
}

// countMatches implements the :count match type (RFC 5231 §4.2): the number
// of entities, written in decimal, is the left side of the relation and each
// key a right side; the test is true if any key satisfies it.
func countMatches(n int, relation, comparator string, keys []string) bool {
	return match.AnyValue([]string{strconv.Itoa(n)}, relation, comparator, keys)
}

// headerFieldCount is the header test's :count (RFC 5231 §4.2): the total
// number of instances of the named fields, summed over the names. A field
// holding several addresses is one instance. A structured field is one
// instance when set; each Raw value is one more.
func headerFieldCount(msg *Message, names []string) int {
	n := 0
	for _, name := range names {
		if !validHeaderName(name) {
			continue
		}
		lower := strings.ToLower(name)
		h := msg.Headers
		var set bool
		switch lower {
		case "subject":
			set = h.Subject != ""
		case "from":
			set = len(formatAddresses(h.From)) > 0
		case "to":
			set = len(formatAddresses(h.To)) > 0
		case "cc":
			set = len(formatAddresses(h.Cc)) > 0
		case "bcc":
			set = len(formatAddresses(h.Bcc)) > 0
		case "message-id":
			set = h.MessageID != ""
		case "in-reply-to":
			set = h.InReplyTo != ""
		case "date":
			set = h.Date != ""
		case "references":
			set = len(h.References) > 0
		}
		if set {
			n++
		}
		for k, vals := range h.Raw {
			if match.ASCIIFold(k) == lower {
				n += len(vals)
			}
		}
	}
	return n
}

// envelopeCount is the envelope test's :count (RFC 5231 §4.2): the number of
// addresses in the named parts. A null reverse-path counts as 0 and any other
// MAIL FROM as 1.
func envelopeCount(msg *Message, parts []string) int {
	n := 0
	for _, name := range parts {
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "from":
			if msg.Envelope.From != "" {
				n++
			}
		case "to":
			n += len(msg.Envelope.To)
		}
	}
	return n
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
	if !msg.NoBody {
		size += 2 // CRLF blank line separating the header block from the body
	}
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

// validHeaderName accepts RFC 5322 ftext only. Invalid test operands never
// match, even if the host supplied an invalid key in Headers.Raw (§2.4.2.2).
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if name[i] < 33 || name[i] > 126 || name[i] == ':' {
			return false
		}
	}
	return true
}

// headerValues returns every value of the named header, drawing from both the
// structured Headers fields and the raw header map (matched with ASCII case
// folding: field names are RFC 5322 ftext, not Unicode identifiers).
func headerValues(msg *Message, name string) []string {
	if !validHeaderName(name) {
		return nil
	}
	lower := strings.ToLower(name)
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
		if match.ASCIIFold(k) == lower {
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

// unfoldAndTrim applies RFC 5322 §2.2.3 unfolding to a header value:
// remove only CRLF immediately followed by SP or HTAB, preserving the folding
// whitespace itself. Then strip leading and trailing mail whitespace (SP, HTAB
// and line breaks), as RFC 5228 §5.7 requires the header test to ignore.
// Non-folding line breaks, other controls and Unicode spaces remain header
// content. Values without a CRLF take a fast path that only trims.
func unfoldAndTrim(v string) string {
	if !strings.Contains(v, "\r\n") {
		return strings.Trim(v, " \t\r\n")
	}
	var b strings.Builder
	b.Grow(len(v))
	for i := 0; i < len(v); i++ {
		if v[i] == '\r' && i+2 < len(v) && v[i+1] == '\n' && (v[i+2] == ' ' || v[i+2] == '\t') {
			i++ // Remove CRLF only; copy all following whitespace unchanged.
			continue
		}
		b.WriteByte(v[i])
	}
	return strings.Trim(b.String(), " \t\r\n")
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

func gatherAddressValues(msg *Message, names []string, part string, sub Subaddress) []string {
	var out []string
	for _, n := range names {
		for _, addr := range addressList(msg, n) {
			if v, ok := addressPartOf(addr, part, sub); ok {
				out = append(out, v)
			}
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
	if !validHeaderName(name) {
		return nil
	}
	lower := strings.ToLower(name)
	// RFC 5228 §5.1 restricts this test to address-bearing fields. A Subject
	// that happens to look like an addr-spec is still unstructured text.
	switch lower {
	case "from", "to", "cc", "bcc", "sender", "reply-to",
		"resent-from", "resent-to", "resent-cc", "resent-bcc", "resent-sender":
	default:
		return nil
	}
	var out []string
	switch lower {
	case "from":
		out = append(out, addressSpecs(msg.Headers.From)...)
	case "to":
		out = append(out, addressSpecs(msg.Headers.To)...)
	case "cc":
		out = append(out, addressSpecs(msg.Headers.Cc)...)
	case "bcc":
		out = append(out, addressSpecs(msg.Headers.Bcc)...)
	}
	for k, vals := range msg.Headers.Raw {
		if match.ASCIIFold(k) != lower {
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

func gatherEnvelopeValues(msg *Message, parts []string, part string, sub Subaddress) []string {
	var out []string
	for _, name := range parts {
		for _, v := range envelopeValues(msg, name) {
			if p, ok := addressPartOf(dropSourceRoute(v), part, sub); ok {
				out = append(out, p)
			}
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

// dropSourceRoute removes the obsolete SMTP route before comparing any
// envelope part (RFC 5228 §5.4). A colon elsewhere in an addr-spec is data.
func dropSourceRoute(addr string) string {
	candidate := addr
	if strings.HasPrefix(candidate, "<") && strings.HasSuffix(candidate, ">") {
		candidate = candidate[1 : len(candidate)-1]
	}
	if strings.HasPrefix(candidate, "@") {
		inLiteral := false
		for i := 0; i < len(candidate); i++ {
			switch candidate[i] {
			case '[':
				inLiteral = true
			case ']':
				inLiteral = false
			case ':':
				if !inLiteral {
					return candidate[i+1:]
				}
			}
		}
	}
	return addr
}

// addressPartOf extracts the requested part (:all/:localpart/:domain, or the
// subaddress :user/:detail under sub) from an address. For an address without
// an "@", the whole string is the local part and the domain is empty. ok is
// false only for :detail of an address that encodes no detail, which then
// matches no key at all (RFC 5233 §4).
func addressPartOf(addr, part string, sub Subaddress) (value string, ok bool) {
	// The last @ may be dtext inside a bracketed domain literal (RFC
	// 2822 §3.4.1). Locate the separator before that literal, while still
	// allowing @ inside a quoted local-part. Structured addresses and
	// envelope values are already validated by the host.
	i := strings.LastIndexByte(addr, '@')
	if strings.HasSuffix(addr, "]") {
		if literal := strings.LastIndex(addr, "@["); literal >= 0 {
			i = literal
		}
	}
	local := addr
	if i >= 0 {
		local = addr[:i]
	}
	switch part {
	case ":localpart":
		return local, true
	case ":domain":
		if i >= 0 {
			return addr[i+1:], true
		}
		return "", true
	case ":user":
		if user, _, has := splitSubaddress(local, sub); has {
			return user, true
		}
		// No detail encoded: :user is the whole local part (RFC 5233 §4).
		return local, true
	case ":detail":
		_, detail, has := splitSubaddress(local, sub)
		return detail, has
	default: // :all
		return addr, true
	}
}

// splitSubaddress splits a local part into user and detail at the first
// occurrence of the separator (RFC 5233 §4 leaves repeated separators to the
// implementation). has is false when the local part encodes no detail: there
// is no separator, subaddressing is off, or the detail would be empty and the
// mail system does not allow that.
func splitSubaddress(local string, sub Subaddress) (user, detail string, has bool) {
	if sub.Separator == "" {
		return "", "", false
	}
	before, after, found := strings.Cut(local, sub.Separator)
	if !found {
		return "", "", false
	}
	user, detail = before, after
	if sub.DetailFirst {
		user, detail = after, before
	}
	if detail == "" && sub.NoEmptyDetail {
		return "", "", false
	}
	return user, detail, true
}

// ── Matching ─────────────────────────────────────────────────────────

// ── Body text extraction ─────────────────────────────────────────────

// bodyValues returns the strings a body test matches against. With :content
// (RFC 5173 §5.2) each MIME part whose type matches the list is a separate
// string, so a key never matches across part boundaries. :raw (§5.1) is the
// host's undecoded body as one string, and :text the extracted text.
func bodyValues(msg *Message, tt *bodyTest) []string {
	switch tt.transform {
	case ":content":
		budget := maxBodyParts
		return contentPartsBounded(nil, msg.Body, tt.contentTypes, maxBodyDepth, &budget)
	case ":raw":
		if !msg.RawBodySet {
			panic(ErrRawBodyUnavailable)
		}
		return []string{msg.RawBody}
	}
	return []string{extractBodyText(msg)}
}

// contentPartsBounded appends the content of b and of every part nested in it
// whose type matches one of specs. The walk descends into every part, matching
// or not, and stops at the same depth and part bounds as the other body walks.
func contentPartsBounded(out []string, b Body, specs []string, depthLeft int, budget *int) []string {
	if depthLeft <= 0 || *budget <= 0 {
		return out
	}
	*budget--
	for _, spec := range specs {
		if contentTypeMatches(spec, b.ContentType) {
			out = append(out, b.Content)
			break
		}
	}
	for _, p := range b.Parts {
		out = contentPartsBounded(out, p, specs, depthLeft-1, budget)
	}
	return out
}

// contentTypeMatches reports whether a :content type matches a part's
// Content-Type (RFC 5173 §5.2). The empty string matches every type; a bare
// type matches each of its subtypes; "type/subtype" matches only itself; a
// spec that begins or ends with a slash, or has more than one, matches none.
// A part without a Content-Type is text/plain (RFC 2045 §5.2).
func contentTypeMatches(spec, contentType string) bool {
	if spec == "" {
		return true
	}
	if strings.HasPrefix(spec, "/") || strings.HasSuffix(spec, "/") || strings.Count(spec, "/") > 1 {
		return false
	}
	mediaType, _, _ := strings.Cut(contentType, ";")
	// MIME type tokens are US-ASCII (RFC 2045 §5.1). Unicode case folding
	// would equate, for example, text/plaİn with text/plain.
	mediaType = match.ASCIIFold(strings.TrimSpace(mediaType))
	if mediaType == "" {
		mediaType = "text/plain"
	}
	spec = match.ASCIIFold(spec)
	if strings.Contains(spec, "/") {
		return mediaType == spec
	}
	typ, _, _ := strings.Cut(mediaType, "/")
	return typ == spec
}

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
