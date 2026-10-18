# Coverage exceptions

CI fails when statement coverage of `./...` falls below 95%. Coverage is 97.8%
(the package alone 97.6%); the statements below are the ones left uncovered.
Add an entry here whenever you leave a statement uncovered.

## Left uncovered on purpose

| File | Function | Statements | Reason |
|------|----------|------------|--------|
| `internal/checklist/main.go` | `main` | `os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))` | Exiting the process cannot be tested in-process. All of the checker's logic is in `run`, which the tests cover; CI also runs the command itself. |

## Not covered yet

These are known gaps, not exclusions: error returns that propagate a failure of
a helper (`return nil, err` after `expect` or `parseStringList` in
`parseIf`, `parseBlock`, `parseTestList`, `parseSizeTest`, `parseFileinto`,
`parseNotify`, `parseBodyTest`), the helper itself being
covered through other callers; `require` inside a block (`parseRequire`), a
second match type or body transform and the `:content` argument (`parseMatchOptions`), and a few
defensive branches of the lexer and of `decodeEncodedCharacters` and
`asciiNumeric`. The same table will shrink as the extension requirements
(RFC 5230, 5232, 5173, 3894, 5490) are checked item by item.

The RFC 5230 audit covers all statements in `parseVacation`, `vacationReplyTo`
and `vacationAllowed`. The local Pi cannot start the race detector
(`ThreadSanitizer: unsupported VMA range`, found 47, supported 48), so the figures
above were measured with atomic coverage without `-race`; CI retains the pinned
race-coverage workflow on its supported runner.
