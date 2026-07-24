package sieve_test

import (
	"fmt"

	sieve "github.com/rest-mail/go-sieve"
)

// mailbox is a minimal sieve.Executor that records where the script decided to
// deliver the message and which flags it set. A real implementation would move
// the message into an actual folder, persist IMAP flags, send the vacation
// reply, and so on — the package decides what to do, this type decides how.
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

// Example parses a small Sieve script, runs it against a message, and inspects
// the actions the script selected.
func Example() {
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
		fmt.Printf("deliver to %q with flags %v\n", mb.folder, mb.flags)
	}
	// Output: deliver to "Invoices" with flags [\Flagged]
}
