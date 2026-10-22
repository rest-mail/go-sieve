# Coverage exceptions

CI fails when statement coverage of `./...` falls below 95%. CI measured 99.5%
(the package alone 99.6%) on main before the lexer gaps below were closed; the
statements below are the ones left uncovered. Add an entry here whenever you
leave a statement uncovered.

## Left uncovered on purpose

| File | Function | Statements | Reason |
|------|----------|------------|--------|
| `internal/checklist/main.go` | `main` | `os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))` | Exiting the process cannot be tested in-process. All of the checker's logic is in `run`, which the tests cover; CI also runs the command itself. |

## Not covered yet

No known gaps remain in the lexer (`tokenize` through `readMultiLineString`).
`TestMultiLineString_HashCommentAfterMarker` covers the hash comment that may
end the `text:` line of a multi-line string (RFC 5228 §8.1), including malformed
scripts where the comment ends the source, the closing `.` is missing, a word
stands before the comment or a bracket comment stands in for it.
`TestLexer_Errors` and `TestRFC5228_SyntaxErrors` cover unexpected characters,
unterminated strings and bracket comments, and numbers that overflow.
`parseSieveScript` no longer checks for a token left after the top-level
commands: `parseCommands(false)` returns without an error only at the end of
the script, so the check could not fire. `TestParse_TopLevelLeftoverTokens`
shows every kind of leftover token is still refused.
`TestRFC5228SourceNULRejected` and `TestRFC5228SourceNULDiagnostic` cover
whole-source NUL rejection and its line diagnostic in `tokenize`; the now
redundant quoted-string NUL branches were removed.
`TestRFC5228SourceNonNULOctets` preserves every legal non-linebreak octet in
comments and multiline strings, and encoded NUL values.
`TestQuotedString_EscapedOctets` covers the remaining escaped-octet rejection
in `readQuotedString` against all 256 octets, and
`TestQuotedString_EscapedLineBreakAndTruncation` covers incomplete escapes.
Every statement of `eval.go` is covered:
`TestSize_ReconstructionCountsRawHeaders` and `TestSize_ReconstructionPartBudget`
cover raw headers and the part budget in the reconstructed size,
`TestAddress_RawHeaderOnlyNamed` the raw-header name filter of `addressList`, and
`TestEvalTest_UnknownIsFalse` the fail-closed end of `evalTest`. `matchOne` has
no unreachable fallback any more: `:is` is its default case.
`TestEncodedCharacter_SequenceErrors` covers every statement of
`decodeEncodedCharacters`.
`TestEncodedCharacterUnicodeMalformedRangeLiteral` and
`TestEncodedCharacterUnicodeMalformedRangeOctets` cover deferred Unicode range
errors: malformed payloads stay literal, a well-formed payload reports its first
invalid scalar, and independently valid neighboring encodings still decode.
`TestEncodedCharacterBlankGrammar` covers SP/HTAB/CRLF separators and malformed
line endings in both hex and Unicode payloads.
`asciiNumeric` no longer has a gap: it keeps the digit run as a string, so a
number too long for a uint64 cannot fail to parse.
`TestASCIINumericFirstOctet` checks every possible initial octet through public
body evaluation, and `TestASCIINumericRFCExamplesAndPrefixes` checks numeric
termination, non-digit prefixes and long values without whitespace trimming.
`TestParse_NestedErrorsPropagate` covers the error returns that hand on a
failure of a nested test, block, string list or expected token in `parseIf`,
`parseBlock`, `parseTestList`, `parseSizeTest`, `parseFileinto`, `parseNotify`,
`parseBodyTest` and `parseRequire`, and a second match type after `:regex` in
`parseMatchOptions`.

The RFC 5230 audit covers all statements in `parseVacation`, `vacationReplyTo`
and `vacationAllowed`. The local Pi cannot start the race detector
(`ThreadSanitizer: unsupported VMA range`, found 47, supported 48), so the figures
above were measured with atomic coverage without `-race`; CI retains the pinned
race-coverage workflow on its supported runner.

`TestRFC5228HeaderTrimPreservesNonWSP` and
`TestRFC5228HeaderTrimBoundaryOctets` cover the mail-whitespace trimming in both
paths of `unfoldAndTrim`, including raw and structured headers, non-ASCII spaces,
opaque octets, and controls that are not mail whitespace. No coverage exception
is added; the shared CI coverage floor remains 95%.
