# go-sieve

[![CI](https://github.com/rest-mail/go-sieve/actions/workflows/ci.yml/badge.svg)](https://github.com/rest-mail/go-sieve/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rest-mail/go-sieve.svg)](https://pkg.go.dev/github.com/rest-mail/go-sieve)
[![Go Report Card](https://goreportcard.com/badge/github.com/rest-mail/go-sieve)](https://goreportcard.com/report/github.com/rest-mail/go-sieve)

A parser and interpreter for the Sieve mail-filtering language
([RFC 5228](https://www.rfc-editor.org/rfc/rfc5228)) in Go — standard library
only, no external dependencies.

## About

Sieve is the language mail servers use to let users sort, file, redirect, and
auto-reply to incoming mail without running arbitrary code. This package turns a
Sieve script into an executable decision over one message.

Parsing and evaluation are decoupled from any mailbox model. You `Parse` a script
once into a `*Script`, adapt your message into the neutral `Message` type, and
call `Script.Evaluate` with an `Executor` you implement. The evaluator walks the
script, evaluates its tests against the message, and calls your Executor's
methods to apply the actions the script selected — how each action maps onto a
real mailbox (folder names, flag storage, vacation de-duplication, notification
transport) is entirely your concern. The package decides *what* to do; your host
decides *how*. The two terminal dispositions, `discard` and `reject`, are
reported through the returned `Outcome` rather than the Executor.

Delivery follows RFC 5228's implicit-keep model: unless the script cancels it
(with `keep`, `fileinto`, `redirect`, or `discard`), the message is delivered to
the default mailbox, reported as `Outcome.ImplicitKeep`. A `discard` only cancels
that keep — it does not stop the script, so any other delivering action still
takes effect. A runtime error during evaluation fails safe to the implicit keep
(`Outcome.Error`) rather than losing the message.

## Features

- **Control**: `if` / `elsif` / `else`, `require`, `stop`.
- **Tests**: `address`, `header`, `envelope`, `exists`, `size` (`:over`/`:under`),
  `body`, `allof`, `anyof`, `not`, `true`/`false`.
- **Actions**: `keep`, `discard`, `fileinto` (+`:create`), `redirect`, `reject`,
  `imap4flags` (`setflag`/`addflag`/`removeflag`), `vacation`, `notify`.
- **Match types**: `:is`, `:contains`, `:matches` (glob with `*` and `?`), plus a
  non-standard `:regex`. **Comparators**: `i;ascii-casemap` (default),
  `i;octet`, `i;ascii-numeric`. **Address parts**: `:all`/`:localpart`/`:domain`.
- **Strict where it counts, lenient elsewhere**: the parser is strict about the
  constructs it understands (so `Validate` catches real mistakes) but skips
  unknown commands, tests, and tagged arguments, so scripts using extensions this
  package does not implement still load and run their recognised parts.
- Zero external dependencies.

## Install

```sh
go get github.com/rest-mail/go-sieve
```

## Quickstart

Parse a script, run it against a message, and inspect the actions it selected.
Your `Executor` maps each action onto your delivery model; the terminal
`discard`/`reject` decisions come back in the `Outcome`.

```go
package main

import (
	"fmt"

	sieve "github.com/rest-mail/go-sieve"
)

// mailbox implements sieve.Executor, applying actions to your delivery model.
type mailbox struct {
	folder string
	flags  []string
}

func (m *mailbox) Keep()                               {}
func (m *mailbox) FileInto(folder string, create bool) { m.folder = folder }
func (m *mailbox) Redirect(addr string)                {}
func (m *mailbox) Flag(op string, flags []string)      { m.flags = flags }
func (m *mailbox) Vacation(v sieve.Vacation)           {}
func (m *mailbox) Notify(method, message string)       {}

func main() {
	script, err := sieve.Parse(`require ["fileinto", "imap4flags"];
if header :contains "Subject" "invoice" {
    setflag "\\Flagged";
    fileinto :create "Invoices";
}`)
	if err != nil {
		panic(err)
	}

	msg := &sieve.Message{
		Headers: sieve.Headers{Subject: "Your March invoice is ready"},
	}

	mb := &mailbox{}
	outcome := script.Evaluate(msg, mb)

	switch outcome.Disposition {
	case sieve.Discard:
		fmt.Println("discard")
	case sieve.Reject:
		fmt.Println("reject:", outcome.RejectReason)
	default:
		if outcome.ImplicitKeep {
			mb.Keep() // implicit keep: deliver to the default mailbox
		}
		fmt.Printf("deliver to %q with flags %v\n", mb.folder, mb.flags)
	}
	// Prints: deliver to "Invoices" with flags [\Flagged]
}
```

Check a script's syntax without evaluating it with `sieve.Validate(src)`.

## The Executor

`Executor` is the seam between the language and your mailbox. Its methods
(`Keep`, `FileInto`, `Redirect`, `Flag`, `Vacation`, `Notify`) are invoked in
script order as each non-terminal action fires; you decide what a folder name,
flag, redirect, or vacation reply actually means for your storage and transport.
The terminal dispositions are not methods — `reject` refuses the message and
`discard` (when nothing else delivers it) drops it, and both surface through
`Outcome.Disposition` (with `Outcome.RejectReason` for a reject). A `discard`
alongside a delivering action does not drop the message; it only cancels the
implicit keep, so `Outcome.ImplicitKeep` tells you whether to also deliver to the
default mailbox. For a `vacation` action the evaluator computes
`Vacation.ReplyTo` (the envelope sender, falling back to the `From` header) and
reports the minimum reply interval as `Vacation.Interval` (a duration, so a
`:seconds` argument keeps its precision); a message with a null reverse-path
(`MAIL FROM:<>`) gets no `Vacation` call at all. De-duplication and actually
sending the auto-reply remain your responsibility.

Adapt your own email representation into the neutral `Message` before evaluating:
`Headers` carries the common structured fields plus a `Raw` map consulted
(case-insensitively) for any other header, so custom headers such as `X-Priority`
are testable; `Envelope` supplies the SMTP identities the `envelope` test reads;
`Body` and `Attachments` feed the `body` and `size` tests.

## Documentation

Full API reference:
[pkg.go.dev/github.com/rest-mail/go-sieve](https://pkg.go.dev/github.com/rest-mail/go-sieve).

## Changelog

Recent releases — see [CHANGELOG.md](CHANGELOG.md) for the complete history.

- **v0.3.0** (2026-07-25) — breaking: evaluation `Outcome.ImplicitKeep` host-contract (fixes discard mail-loss) + `Vacation.Days`→`Interval`; comparator and encoded-character conformance.
- **v0.2.0** (2026-07-25) — breaking: `require` is now enforced; parser recursion bounded and redirect-target/size-quantifier hardening.
- **v0.1.1** (2026-07-23) — documentation and packaging polish for the initial release.
- **v0.1.0** (2026-07-23) — initial public release: RFC 5228 Sieve parser and evaluator.

## License

[MIT](LICENSE) © 2026 rest-mail
