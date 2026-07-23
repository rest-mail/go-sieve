# sieve

[![CI](https://github.com/rest-mail/go-sieve/actions/workflows/ci.yml/badge.svg)](https://github.com/rest-mail/go-sieve/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/rest-mail/go-sieve.svg)](https://pkg.go.dev/github.com/rest-mail/go-sieve)

A parser and interpreter for the Sieve mail-filtering language
([RFC 5228](https://www.rfc-editor.org/rfc/rfc5228)) in Go, with zero external
dependencies (standard library only).

Parsing and evaluation are decoupled from any mailbox model. You `Parse` a script
once into a `*Script`, adapt your message into the neutral `Message` type, and
call `Script.Evaluate` with an `Executor` you implement. The evaluator walks the
script, evaluates its tests against the message, and calls your Executor's
methods to apply the actions the script selected — how each action maps onto a
real mailbox (folder names, flag storage, vacation de-duplication, notification
transport) is entirely your concern. The two terminal dispositions, `discard`
and `reject`, are reported through the returned `Outcome`.

Supported:

- **Control**: `if` / `elsif` / `else`, `require`, `stop`.
- **Tests**: `address`, `header`, `envelope`, `exists`, `size` (`:over`/`:under`),
  `body`, `allof`, `anyof`, `not`, `true`/`false`.
- **Match types**: `:is`, `:contains`, `:matches` (glob), and a non-standard
  `:regex`. **Comparators**: `i;ascii-casemap` (default), `i;octet`,
  `i;ascii-numeric`. **Address parts**: `:all`/`:localpart`/`:domain`.
- **Actions / extensions**: `keep`, `discard`, `fileinto` (+`:create`),
  `redirect`, `reject`, `imap4flags` (`setflag`/`addflag`/`removeflag`),
  `vacation`, `notify`.

Parsing is strict about the constructs it understands (so `Validate` catches
real mistakes) but lenient about unknown commands, tests, and tagged arguments,
so scripts using extensions this package does not implement still load and run
their recognised parts.

## Install

```sh
go get github.com/rest-mail/go-sieve
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/rest-mail/go-sieve"
)

// mailbox implements sieve.Executor, applying actions to your delivery model.
type mailbox struct{ folder string }

func (m *mailbox) Keep()                              {}
func (m *mailbox) FileInto(folder string, create bool) { m.folder = folder }
func (m *mailbox) Redirect(addr string)               {}
func (m *mailbox) Flag(op string, flags []string)     {}
func (m *mailbox) Vacation(v sieve.Vacation)          {}
func (m *mailbox) Notify(method, message string)      {}

func main() {
	script, err := sieve.Parse(`require "fileinto";
if header :contains "Subject" "invoice" { fileinto "Invoices"; }`)
	if err != nil {
		panic(err)
	}

	msg := &sieve.Message{
		Headers: sieve.Headers{Subject: "Your invoice is ready"},
	}

	mb := &mailbox{}
	outcome := script.Evaluate(msg, mb)
	switch outcome.Disposition {
	case sieve.Discard:
		fmt.Println("discard")
	case sieve.Reject:
		fmt.Println("reject:", outcome.RejectReason)
	default:
		fmt.Println("deliver to:", mb.folder) // "Invoices"
	}
}
```

Validate a script without running it with `sieve.Validate(src)`.

## License

[MIT](LICENSE) © 2026 rest-mail
