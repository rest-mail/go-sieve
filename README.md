# go-sieve

[![CI](https://github.com/rest-mail/go-sieve/actions/workflows/ci.yml/badge.svg)](https://github.com/rest-mail/go-sieve/actions/workflows/ci.yml)
[![Coverage gate: >=95%](https://img.shields.io/badge/coverage%20gate-%3E%3D95%25-brightgreen)](COVERAGE.md)
[![Checklist: 159/244 mapped](https://img.shields.io/badge/checklist-159%2F244%20mapped-blue)](CHECKLIST.md)
[![Go Reference](https://pkg.go.dev/badge/github.com/rest-mail/go-sieve.svg)](https://pkg.go.dev/github.com/rest-mail/go-sieve)
[![Go Report Card](https://goreportcard.com/badge/github.com/rest-mail/go-sieve)](https://goreportcard.com/report/github.com/rest-mail/go-sieve)

A parser and interpreter for the Sieve mail-filtering language
([RFC 5228](https://www.rfc-editor.org/rfc/rfc5228)) in Go. The library imports
only the standard library.

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
the default mailbox, reported as `Outcome.ImplicitKeep`. A `fileinto` or
`redirect` tagged `:copy` (RFC 3894) leaves the implicit keep in effect, but does
not bring it back once another action has cancelled it. A `discard` only cancels
that keep — it does not stop the script, so any other delivering action still
takes effect. A runtime error during evaluation fails safe to the implicit keep
(`Outcome.Error`) rather than losing the message.

## Features

- **Control**: `if` / `elsif` / `else`, `require`, `stop`.
- **Tests**: `address`, `header`, `envelope`, `exists`, `size` (`:over`/`:under`),
  `body`, `allof`, `anyof`, `not`, `true`/`false`.
- **Actions**: `keep`, `discard`, `fileinto` (+`:create`), `redirect`, `reject`,
  `imap4flags` (`setflag`/`addflag`/`removeflag`), `vacation` (including
  RFC 6131 `vacation-seconds`), and the local `notify` action.
- **Match types**: `:is`, `:contains`, `:matches` (glob with `*` and `?`), plus a
  non-standard `:regex`. **Comparators**: `i;ascii-casemap` (default),
  `i;octet`, `i;ascii-numeric` (a numeric value must begin with an ASCII digit;
  leading whitespace makes it positive infinity). **Address parts**:
  `:all`/`:localpart`/`:domain`.
- **Extension validation**: undeclared extensions and unknown commands, tests,
  and tagged arguments are errors. The RFC 5490 `mailbox` capability currently
  supports `fileinto :create` only; `mailboxexists` is not implemented.
  `mboxmetadata`, `servermetadata`, `relational`, `subaddress`, `variables`
  and standardized `enotify` are not supported. Local `regex` uses Go regular
  expressions; local `notify` is not RFC 5435 conformance.
- No external dependencies in the library. The tests alone use
  [foxcpp/go-sieve](https://github.com/foxcpp/go-sieve) (MIT) as an
  independent reference: `diff_test.go` runs the same scripts on the same
  message through both and compares where the message is delivered.

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
func (m *mailbox) Redirect(addr string, copy bool)     {}
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

For `FileInto(folder, create)`, a true `create` asks the host to create the
mailbox before delivery if it is missing, or use it unchanged if it exists
(RFC 5490 §3.2). The library does not inspect the mailstore. A creation failure
must be treated as a runtime error: the callback can panic with an error, which
`Evaluate` returns as `Outcome.Error` with `Outcome.ImplicitKeep` set. The
callback has no error return, so a failure recorded only in host state is not
visible to the evaluator.

`Executor` is the seam between the language and your mailbox. Its methods
(`Keep`, `FileInto`, `Redirect`, `Flag`, `Vacation`, `Notify`) are invoked in
script order as each non-terminal action fires; you decide what a folder name,
flag, redirect, or vacation reply actually means for your storage and transport.
The terminal dispositions are not methods — `reject` refuses the message and
`discard` (when nothing else delivers it) drops it, and both surface through
`Outcome.Disposition` (with `Outcome.RejectReason` for a reject). A `discard`
alongside a delivering action does not drop the message; it only cancels the
implicit keep, so `Outcome.ImplicitKeep` tells you whether to also deliver to the
default mailbox. `Redirect(addr, copy)` reports each redirect's own `:copy` tag;
forward the message either way, and leave the keep decision to
`Outcome.ImplicitKeep` (a script-wide `require "copy"` says nothing about any one
redirect). For a `vacation` action the evaluator reports the SMTP envelope
sender in `Vacation.ReplyTo` and the minimum reply interval in
`Vacation.Interval`. `:days` defaults to 7 and is clamped to 1..365 days before
conversion to a duration. `:seconds` requires `require "vacation-seconds"`,
which also enables base `vacation`. It retains second precision, including zero,
and caps at 2147483647 seconds before duration conversion; `:days` and `:seconds`
together are an error. A missing or null
reverse-path gets no callback. Neither do messages not addressed personally to
an envelope recipient or a `:addresses` alias, mailing-list and automated mail,
bulk/list/junk mail, or excluded system senders. Supply only the evaluated user's
final envelope recipient identities and preserve the original recipient and
automation headers in `Message`.
Only one vacation action may execute per evaluation; a second returns an error
and the implicit keep, including when the first action was suppressed.

The callback preserves `Subject`, `From`, `Body`, `MIME`, and `Handle`.
`SubjectSet` and `HandleSet` distinguish explicit empty strings from omitted
tags. The host owns persistent tracking: use the explicit handle when
`HandleSet`, otherwise the original subject/from/MIME/reason arguments, together
with the recipient user and sender. Do not include a generated default subject
in that identity, and honor `Interval` when sending. The host also validates
and serializes MIME, encodes non-ASCII subjects, generates defaults and threading
headers, and sends the reply with `Auto-Submitted: auto-replied` and a null SMTP
reverse-path. See the separate RFC 5230 section in [CHECKLIST.md](CHECKLIST.md)
for the complete library/host contract.

Adapt your own email representation into the neutral `Message` before evaluating:
`Headers` carries the common structured fields plus a `Raw` map consulted
(using ASCII case folding) for any other header, so custom headers such as `X-Priority`
are testable; `Envelope` supplies the SMTP identities the `envelope` test reads;
`Body` and `Attachments` feed the `body` and `size` tests. Set `NoBody` for a
message that is only a header, with no empty line after it: every `body` test of
it is false, even one for the empty string (RFC 5173 §4).

The RFC 5228 core audit also records host duties and known gaps: MIME header
charset preparation, redirect policy/trace/loop controls, strict whole-script
line endings and octets, and full structured-address validation. Redirect
targets are checked after encoded-character decoding as a single mailbox using
Go `net/mail`, with groups and source routes refused. The original valid value
is passed unchanged to `Executor.Redirect`. This accepts named and quoted
mailboxes and any non-empty dtext domain literal (IPv4, `IPv6:` or other),
checked the same way on every Go version; obsolete address forms and folding
whitespace inside literals remain a known grammar gap (SIEVE-CORE-45).
See the dated audit in [CHECKLIST.md](CHECKLIST.md). Address tests recognize
From, To, Cc, Bcc, Sender, Reply-To and their Resent variants; unstructured and
unregistered custom fields are available through `header`, not `address`.

## The RFC checklist

[CHECKLIST.md](CHECKLIST.md) lists the requirements of RFC 5228 item by item,
and a small program, [`internal/checklist`](internal/checklist/main.go), checks
it against the tests in CI. It is the same program as
[go-spf](https://github.com/rest-mail/go-spf)'s,
[go-mtasts](https://github.com/rest-mail/go-mtasts)'s and the other `go-*`
libraries'.

The checklist is a Markdown file holding one or more tables whose header is
exactly:

```markdown
| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-CMD-04 | 3.3 | Normative | `stop` ends the script, from any depth of nesting. | `TestSieve_Stop` |
| SIEVE-ACT-04 | 4.2, 10 | MUST | An implementation keeps a redirect from looping. | n/a: the host's job. |
```

- **ID** is unique, upper case, and ends in a number.
- **Section** is the RFC section (or sections) the requirement comes from.
- **Level** is the RFC 2119 keyword it carries, or `Normative` for normative
  text without a keyword.
- **Requirement** says what is required, in one line (no `|`).
- **Tests** lists the test functions that cover it, separated by commas, or
  says `n/a:` and why it does not apply.

Each test names the items it covers in its doc comment:

```go
// checklist: SIEVE-CMD-04
func TestSieve_Stop(t *testing.T) {
```

`go run ./internal/checklist [-file CHECKLIST.md] [package dir ...]` scans the
`_test.go` files of the given package directories (default `.`) and fails when:

- an ID is used twice, or a row is malformed;
- an item names no test and is not marked `n/a`;
- an item lists a test that does not exist, or that does not name the item;
- a test names an item that does not exist, is `n/a`, or does not list it;
- a `Test` or `Fuzz` function names no item (`Example` functions may).

CI also fails below 95% statement coverage (the statements left uncovered are
listed in [COVERAGE.md](COVERAGE.md)) and fuzzes each fuzz target (`FuzzParse`,
`FuzzDecodeEncodedCharacters`, `FuzzBodyContentTypeSelection`) for 30 seconds.

The coverage badge shows the enforced minimum; measured statement coverage is
printed in each CI run's **Verify RFC** log. The checklist badge counts
requirements mapped to tests over all checklist rows, including the 85 `n/a`
rows with documented reasons. CI checks the badge against the actual checklist
and its test mappings. The CI badge reports whether verification passed.

## Documentation

Full API reference:
[pkg.go.dev/github.com/rest-mail/go-sieve](https://pkg.go.dev/github.com/rest-mail/go-sieve).

## Changelog

Recent releases — see [CHANGELOG.md](CHANGELOG.md) for the complete history.

- **Unreleased** — checked against RFC 5228 item by item; a string list with a trailing comma is now rejected; an RFC checklist, a 95% coverage gate and fuzzing.
- **v0.3.0** (2026-07-25) — breaking: evaluation `Outcome.ImplicitKeep` host-contract (fixes discard mail-loss) + `Vacation.Days`→`Interval`; comparator and encoded-character conformance.
- **v0.2.0** (2026-07-25) — breaking: `require` is now enforced; parser recursion bounded and redirect-target/size-quantifier hardening.
- **v0.1.1** (2026-07-23) — documentation and packaging polish for the initial release.
- **v0.1.0** (2026-07-23) — initial public release: RFC 5228 Sieve parser and evaluator.

## License

[MIT](LICENSE) © 2026 rest-mail
