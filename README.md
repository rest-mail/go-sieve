# go-sieve

[![CI](https://github.com/rest-mail/go-sieve/actions/workflows/ci.yml/badge.svg)](https://github.com/rest-mail/go-sieve/actions/workflows/ci.yml)
[![Coverage gate: >=95%](https://img.shields.io/badge/coverage%20gate-%3E%3D95%25-brightgreen)](COVERAGE.md)
[![Checklist: 164/249 mapped](https://img.shields.io/badge/checklist-164%2F249%20mapped-blue)](CHECKLIST.md)
[![Go Reference](https://pkg.go.dev/badge/github.com/rest-mail/go-sieve.svg)](https://pkg.go.dev/github.com/rest-mail/go-sieve/pkg/sieve)
[![Go Report Card](https://goreportcard.com/badge/github.com/rest-mail/go-sieve)](https://goreportcard.com/report/github.com/rest-mail/go-sieve)

A parser and interpreter for the Sieve mail-filtering language
([RFC 5228](https://www.rfc-editor.org/rfc/rfc5228)) in Go. The library imports
only the standard library. Parse a script once into a `*Script`, adapt a message
into `Message`, then call `Script.Evaluate` with your `Executor`. The library
selects actions; your host applies them to its mailstore and transport.

## Contents

- [Install](#install)
- [Packages](#packages)
- [Quick start](#quick-start)
- [Features](#features)
- [What the library does and what the host must do](#what-the-library-does-and-what-the-host-must-do)
- [RFC conformance](#rfc-conformance): [the checklist](#the-rfc-checklist), [what it does not do](#what-it-does-not-do), [badges and coverage](#badges-and-coverage)
- [Changelog](#changelog)
- [Licence](#licence)

## Install

```sh
go get github.com/rest-mail/go-sieve@latest
```

The module declares Go 1.25. The library has no external dependencies; tests use
[foxcpp/go-sieve](https://github.com/foxcpp/go-sieve) (MIT) as an independent
reference.

## Packages

| Import path | What it holds |
|-------------|---------------|
| `github.com/rest-mail/go-sieve/pkg/sieve` | The public parser and interpreter: `Parse`, `Validate`, `Script`, parser options, message types, `Executor`, `Vacation`, `Outcome` and delivery dispositions. |
| `pkg/sieve/internal/grammar` | RFC 5228 lexical grammar: tokens, quoted and multiline strings, quantified numbers and encoded-character decoding. Internal: not importable by callers. |
| `pkg/sieve/internal/match` | Byte comparators, glob matching and the local regular-expression matcher. Internal: not importable by callers. |
| `pkg/sieve/rfctest` | Black-box RFC and differential tests, using only the public API. |

Upgrading from v0.6 or earlier: the root package is gone. Change the import to
`github.com/rest-mail/go-sieve/pkg/sieve`; public names and behavior are unchanged.
[CHANGELOG.md](CHANGELOG.md#070---2026-10-26) has the migration table.
The parser and interpreter share one package because they operate on the same
private syntax tree; there are no separate client or server roles.

## Quick start

Parse a script, run it against a message, and inspect the actions it selected.
Your `Executor` maps each action onto your delivery model; the terminal
`discard`/`reject` decisions come back in the `Outcome`.

```go
package main

import (
	"fmt"

	"github.com/rest-mail/go-sieve/pkg/sieve"
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
	script, err := sieve.Parse(`require ["fileinto", "imap4flags", "mailbox"];
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

## Features

| Area | Supported behavior |
|------|--------------------|
| Control | `if`, `elsif`, `else`, `require`, `stop`. |
| Core tests | `address`, `header`, `exists`, `size :over/:under`, `allof`, `anyof`, `not`, `true`, `false`. |
| Core actions | `keep`, `discard`, `redirect`; `fileinto` after `require "fileinto"`. |
| Matching | `:is`, `:contains`, byte-level `:matches` (`*`, `?`, backslash escapes); `:all`, `:localpart`, `:domain` address parts. |
| Comparators | `i;ascii-casemap` (default), `i;octet`, `i;ascii-numeric` equality on arbitrarily long digit runs. A non-digit first octet, including leading whitespace, is positive infinity. |
| Envelope and body | `envelope` (RFC 5228), `body` (RFC 5173): extracted `:text` and MIME-type-selected `:content`. `:raw` currently uses extracted text; see the planned gap below. |
| Rejection | `reject` and `ereject` (RFC 5429); `Outcome.ExtendedReject` distinguishes them. |
| Copies and folders | `:copy` on fileinto/redirect (RFC 3894); `fileinto :create` (RFC 5490). The host creates folders. |
| Flags | `setflag`, `addflag`, `removeflag` callbacks with normalized lists (RFC 5232). `hasflag` and delivery `:flags` remain gaps. |
| Vacation | RFC 5230 argument validation and suppression rules; RFC 6131 `vacation-seconds`. Persistent suppression and reply transport belong to the host. |
| Encoded strings | `encoded-character`: `${hex:...}` and `${unicode:...}` (RFC 5228). |
| Local extensions | `regex` uses Go regular expressions; `notify` forwards method/message. These are not standardized regex or RFC 5435 `enotify` conformance. |
| Validation and limits | Unsupported or undeclared extensions, unknown commands/tests/tags and duplicate or inapplicable test tags are errors. Default nesting limit 64, configurable with `WithMaxDepth`; body walks are bounded by depth and part count. |

`Script.Requires` reports declared extensions; `Script.Empty` reports an empty
script. `Validate` accepts the same options as `Parse`. Parsing and evaluation
are decoupled from the host's mailbox representation.

## What the library does and what the host must do

Delivery follows RFC 5228's implicit keep. Unless `keep`, a consuming `fileinto`
or `redirect`, or `discard` cancels it, `Outcome.ImplicitKeep` asks the host to
deliver to the default mailbox. A `:copy` action leaves it in effect but never
restores a cancelled keep. `discard` does not stop the script, and another
delivering action takes precedence over discard. Runtime errors fail safe to the
implicit keep through `Outcome.Error`.

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
line endings, and full structured-address validation. Raw NUL and bare CR
are rejected throughout the source, including comments and multiline strings;
encoded-character may produce either in string values. Bare LF outside quoted
strings remains a known line-ending gap (SIEVE-CORE-44). Redirect
targets are checked after encoded-character decoding as a single mailbox using
Go `net/mail`, with groups and source routes refused. The original valid value
is passed unchanged to `Executor.Redirect`. This accepts named and quoted
mailboxes and any non-empty dtext domain literal (IPv4, `IPv6:` or other),
checked the same way on every Go version; obsolete address forms and folding
whitespace inside literals remain a known grammar gap (SIEVE-CORE-45).
See the dated audit in [CHECKLIST.md](CHECKLIST.md). Address tests recognize
From, To, Cc, Bcc, Sender, Reply-To and their Resent variants; unstructured and
unregistered custom fields are available through `header`, not `address`.

## RFC conformance

### The RFC checklist

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


The checker scans each named directory, without recursing. Run all suites:

```sh
go run ./internal/checklist pkg/sieve pkg/sieve/internal/grammar pkg/sieve/rfctest
```

The RFC suites in `pkg/sieve/rfctest` drive the public API. Tests needing private
parser or evaluator state remain beside `pkg/sieve`; lexical tests and decoder
fuzzing also exercise `pkg/sieve/internal/grammar`. Each Test, Fuzz and Example
from before the split is retained.

`pkg/sieve/rfctest/diff_test.go` compares the same scripts and messages with
foxcpp/go-sieve. Independent reference tests also check wildcard matching,
arbitrary-precision numeric equality and quantified integer literals. They
exercise grammar, overflow boundaries, all token octets and public evaluation;
they do not prove host wire-size accounting or mailstore behavior.

### What it does not do

The checklist records unsupported behavior and host duties as `n/a` with a
reason. That records a boundary or a gap, rather than claiming full conformance.

#### Not supported, by design

| Area | Boundary | Why |
|------|----------|-----|
| Mailstore and transport | No storage, folder creation, script upload service, SMTP delivery, redirect trace fields, loop controls, queue or durable action log. | The host owns accounts, storage and delivery policy; an evaluator cannot infer those from one message. |
| MIME and message adaptation | No wire-message parser, RFC 2047 or charset decoder; structured address and envelope values are trusted. Supply decoded UTF-8 values, validated addresses, only the evaluated user's envelope recipients and `RawSize` for exact size tests. | The neutral message model lets a host reuse its existing message parser and preserve its own wire representation. Undecoded inputs do not provide charset conformance (SIEVE-CORE-04/05/06/40/42). |
| Reply generation and persistence | Vacation and notify are callbacks; no reply serialization, persistent suppression, clock, MIME encoding or sender authorization. | The host owns outgoing identity, transport, shared response history and rate limits. |
| Vacation site minimum | `:days` uses a one-day minimum and 365-day maximum; no site-specific minimum setting. | These finite defaults meet RFC 5230 and bound duration conversion; short intervals use `vacation-seconds`. |
| Arbitrary code | No shell commands or host-language execution. | User filters must select constrained mail actions. |

#### Not planned

| Area | Current boundary |
|------|------------------|
| Metadata | No RFC 5490 `mboxmetadata` or `servermetadata`; no annotation store or permission API. |
| Standardized notifications | No RFC 5435 `enotify` validation, options, tests or transport policy. The local `notify` action carries method/message only. |
| Obsolete redirect address forms | Go `net/mail` validation does not accept every obsolete RFC 2822 addr-spec form or folding whitespace inside domain literals (SIEVE-CORE-45). |

#### Planned

| Gap | Issue |
|-----|-------|
| Track flags, implement `hasflag`, and preserve delivery `:flags`, including duplicate-delivery rules (FLAGS-17/18/19). | [#89](https://github.com/rest-mail/go-sieve/issues/89) |
| Supply an undecoded body for `body :raw` (BODY-07). | [#90](https://github.com/rest-mail/go-sieve/issues/90) |
| Add a user-scoped host query for `mailboxexists` (MAILBOX-08/09/10). | [#91](https://github.com/rest-mail/go-sieve/issues/91) |
| Reject bare LF throughout script source (SIEVE-CORE-44). | [#92](https://github.com/rest-mail/go-sieve/issues/92) |
| Report a host policy skip from Redirect without cancelling the implicit keep (SIEVE-CORE-20). | [#93](https://github.com/rest-mail/go-sieve/issues/93) |
| Relational comparisons (RFC 5231): `:value` with ordered operators and `:count`, beyond today's numeric equality (RELATIONAL-01). | [#95](https://github.com/rest-mail/go-sieve/issues/95) |
| Subaddresses (RFC 5233): `:user` and `:detail`, with the separator supplied by the host (SUBADDRESS-01). | [#96](https://github.com/rest-mail/go-sieve/issues/96) |
| Variables (RFC 5229): `set`, interpolation, match variables and the `string` test; `${name}` stays literal until then (VARIABLES-01). | [#97](https://github.com/rest-mail/go-sieve/issues/97) |

### Badges and coverage

The coverage badge shows the **95% minimum** CI enforces; every run prints the
measured statement coverage. [COVERAGE.md](COVERAGE.md) records the evidence and
remaining uncovered statements. Coverage uses `-coverpkg=./...` so black-box RFC
tests count toward the implementation they exercise. CI runs the race detector,
checklist, workflow fixtures and all seven fuzz targets, bounded by 500000
executions per target.

The checklist badge counts 164 mapped requirements out of 249 rows, including
85 `n/a` rows. `TestReadmeChecklistBadge` validates the actual mappings and exact
badge text; the workflow fixtures check the coverage badge against the gate.
The CI badge reports whether verification passed.

## Changelog

Recent releases; see [CHANGELOG.md](CHANGELOG.md) for the complete history.

### v0.7.0 (2026-10-26)

Breaking import change: public API moves to `pkg/sieve`, with lexical grammar
and matching helpers internal and a public-API RFC test suite. Wildcard and
integer parsing gain independent proofs. The changelog has a migration table.

### v0.6.0 (2026-10-23)

Reject bare CR throughout script source and preserve domain literals containing
`@` in address-part comparisons.

### v0.5.0 (2026-10-23)

RFC and extension audits, independent differential tests, stricter source and
argument validation, byte-accurate matching, body selection and flag normalization.

### v0.4.0 (2026-10-19)

Breaking host contract: `Executor.Redirect` receives `copy bool`. Vacation safety
and tracking arguments, extended rejection outcomes, the RFC checklist,
coverage gate and fuzzing.

### v0.3.0 (2026-07-25)

Breaking host contract: honor `Outcome.ImplicitKeep`; `Vacation.Interval` replaces
`Days`. Encoded-character decoding and comparator conformance.

### v0.2.0 (2026-07-25)

Breaking validation: enforce `require`. Bound parser recursion and reject
redirect control characters and overflowing number quantifiers.

### v0.1.1 (2026-07-23)

Documentation and packaging polish.

### v0.1.0 (2026-07-23)

Initial public release: RFC 5228 parser and evaluator.

## Licence

[MIT](LICENSE) © 2026 rest-mail
