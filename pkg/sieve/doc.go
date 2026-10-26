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
// to the default mailbox, signalled by [Outcome.ImplicitKeep]. A fileinto or
// redirect tagged :copy (RFC 3894 §3) does not cancel it. A discard only
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
