# Coverage exceptions

CI fails when statement coverage of `./...` falls below 95%. Coverage is 98.2%
(the package alone 98.0%); the statements below are the ones left uncovered.
Add an entry here whenever you leave a statement uncovered.

## Left uncovered on purpose

| File | Function | Statements | Reason |
|------|----------|------------|--------|
| `internal/checklist/main.go` | `main` | `os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))` | Exiting the process cannot be tested in-process. All of the checker's logic is in `run`, which the tests cover; CI also runs the command itself. |

## Not covered yet

These are known gaps, not exclusions: a few defensive branches of the lexer and
of `asciiNumeric`. `TestEncodedCharacter_SequenceErrors` covers every statement of
`decodeEncodedCharacters`. `TestParse_NestedErrorsPropagate`
covers the error returns that hand on a failure of a nested test, block, string
list or expected token in `parseIf`, `parseBlock`, `parseTestList`,
`parseSizeTest`, `parseFileinto`, `parseNotify`, `parseBodyTest` and
`parseRequire`, and a second match type after `:regex` in `parseMatchOptions`.

The RFC 5230 audit covers all statements in `parseVacation`, `vacationReplyTo`
and `vacationAllowed`. The local Pi cannot start the race detector
(`ThreadSanitizer: unsupported VMA range`, found 47, supported 48), so the figures
above were measured with atomic coverage without `-race`; CI retains the pinned
race-coverage workflow on its supported runner.
