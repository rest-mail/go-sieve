package sieve

import "testing"

// matchedFolder runs a script that files into "hit" when its test matches, and
// reports whether it did.
func matchedFolder(t *testing.T, test string, msg *Message) bool {
	t.Helper()
	r := runSieve(t, `require ["fileinto", "envelope"]; if `+test+` { fileinto "hit"; }`, msg)
	return folderOf(r) == "hit"
}
