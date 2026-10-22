# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Note: pre-1.0, breaking changes may ship in a minor release.

## [Unreleased]

### Removed

- The unreachable "unexpected token" check after the top-level commands in
  `Parse`. At top level `parseCommands` returns without an error only at the
  end of the script; a stray `}` or other leftover token is already refused
  there with "unexpected }" or "expected a command" (RFC 5228 §8.2).
- The unreachable fallback in `matchOne`, which matched an unknown match type as
  `:contains`. `:is`, the default match type (RFC 5228 §2.7.1), is now its
  default case; the parser accepts no other tag, so behaviour is unchanged.
- The unreachable "require inside a block" check in the parser. A block always
  belongs to an `if`, so the "require must appear before any other command"
  error already refuses it (RFC 5228 §3.2); the message is the only change.
- The unreachable parse-error branch in `${hex:...}` decoding. A hex-pair is one
  or two hex digits, so it always fits an octet; the digits are now converted
  directly. Tests cover leading, trailing and mixed blanks in `${hex:...}` and
  `${unicode:...}`, and mixed-case hex digits (RFC 5228 §2.4.2.4, SIEVE-LEX-06).

### Fixed

- Comparator identifiers now use ASCII case folding (RFC 4790 §3.1).
  Unicode look-alikes such as `İ;octet` are rejected by Parse and Validate,
  including after encoded-character decoding and in unexecuted branches.
  ASCII mixed-case names remain accepted. Checked row SIEVE-MT-10 has
  all-octet tests and a dedicated fuzz target run in CI.

- **Breaking:** Parse and Validate reject raw NUL anywhere in script source,
  including ignored comments and multiline strings (RFC 5228 §2.1). Encoded
  NUL string values remain valid. SIEVE-CORE-37 now has exhaustive insertion
  and legal-octet tests; the remaining bare-line-ending gap is CORE-44.

- **Breaking:** `Parse` and `Validate` reject unknown envelope parts, including
  empty or padded names and unknown entries mixed with `from` or `to` in a list
  (RFC 5228 §5.4). Names are checked after encoded-character decoding and remain
  ASCII case-insensitive, independently of the value comparator. Invalid tests
  are rejected even in branches that would not execute. SIEVE-CORE-24 is now
  mapped to a regression test; the checklist badge is 154/239 mapped (85 n/a).

- **Breaking:** invalid header-name operands no longer match fields, and
  `address` only tests standard address-bearing headers. Required address
  headers now work when supplied only in `Headers.Raw`. Incoming envelope
  source routes are removed before comparisons. Multiline strings preserve
  CRLF and a singly prefixed dot; only doubled leading dots are unstuffed.

- **Breaking:** `vacation :seconds` now requires `vacation-seconds`, which
  implies `vacation` (RFC 6131). Scripts declaring only `vacation` must add or
  substitute `vacation-seconds`. Using both `:days` and `:seconds` is now an
  error. Seconds are capped at 2147483647 before duration conversion, so large
  Sieve numbers cannot overflow into a negative reply interval.

### Added

- RFC 5228 sections 1–14 audited against the core checklist; 43 new rows
  distinguish executable requirements, host/specification duties and known
  gaps. Fourteen focused conformance tests cover numeric and byte boundaries,
  nesting minimums, header/address handling, source routes and multiline text.
  README badge updated to 152/238 mapped (86 n/a).

- Seventeen extension requirement rows, twelve mapped to new tests: the
  capability inventory, local regex behavior, and RFC 6131 seconds grammar,
  interval boundaries and meeting example. Unsupported relational, subaddress,
  variables and standardized notification capabilities are explicitly recorded,
  with host reply-rate policy separate from library behavior. README checklist
  badge updated to 136/195 mapped (59 n/a).

- Tests for the hash comment allowed after `text:` in a multi-line string
  (RFC 5228 §8.1, SIEVE-LEX-05): the comment is not part of the string, and a
  comment that ends the script, a missing closing `.`, a word before the
  comment and a bracket comment in its place are refused. This closes the last
  known coverage gap in the lexer.
- README coverage-gate and RFC checklist badges, checked in CI against the
  enforced coverage minimum and the checklist's actual test mappings.
- Differential tests (`diff_test.go`) against
  [foxcpp/go-sieve](https://github.com/foxcpp/go-sieve), a test-only
  dependency: 37 scripts covering RFC 5228's tests, match types, comparators,
  address parts, control flow and RFC 3894's `:copy` must deliver the message
  to the same places in both, and 11 malformed scripts must be refused by both.
  Where the reference departs from RFC 5228 the expected result is written in
  the test: it drops the implicit keep after `stop` (§3.3), and it accepts a
  stray `}`, an `else` with no `if`, two address parts and an unparenthesised
  test list.
- `Message.NoBody` marks a message that is a header with no empty line after
  it. Every `body` test of such a message is false, even one for the empty
  string, and its reconstructed size has no blank line (RFC 5173 §4,
  BODY-05).
- An RFC 5490 section in `CHECKLIST.md` audits `mailbox` (MAILBOX-01 to
  MAILBOX-12), with tests for `fileinto :create` syntax, capability enforcement,
  callback arguments and fail-safe handling of a host creation error.
  `mailboxexists` is recorded as not supported yet; mailbox creation and access
  checks belong to the host. The README and `Executor.FileInto` document the
  partial capability and the host's creation/error contract.
- An RFC 3894 section in `CHECKLIST.md` checks the `copy` extension clause by
  clause (COPY-01 to COPY-06), with tests for its capability, syntax, the RFC's
  example and a copy taken alongside other actions.
- An RFC 5232 section in `CHECKLIST.md` checks the `imap4flags` extension
  clause by clause (FLAGS-01 to FLAGS-21). `hasflag` and delivering with the
  `:flags` list are recorded as not supported yet.
- An RFC 5173 section in `CHECKLIST.md` checks the `body` extension clause by
  clause (BODY-01 to BODY-18). `:raw` matching the undecoded body and telling a
  missing body from an empty one are recorded as not supported yet.

### Changed

- **`Executor.Flag` receives a cleaned-up list of flags** (RFC 5232 §2). A
  string holding several space-separated flags is split into them, empty names
  are dropped, a name that is not a valid IMAP flag (non-ASCII, or holding
  `( ) { % * " \ ]` or a control character) is dropped, `\Recent` and other
  system flags a client cannot set are dropped, the five settable system flags
  are written in their usual case, and a repeated flag is passed once. Hosts
  that did this themselves can stop.
- **`body :content` matches the parts it names** (RFC 5173 §5.2). The list of
  content types was parsed and ignored, so `:content "text/html"` matched the
  plain-text part. Each part of a listed type (`""` for all, `"text"` for any
  subtype, `"text/html"` for one) is now matched on its own, so a key never
  spans two parts. `:content` also takes a string list, as the RFC says, rather
  than one string. `Body.Content` is documented as decoded UTF-8 text.

### Fixed

- Quoted strings reject backslash-escaped NUL, CR and LF (RFC 5228 §8.1).
  These octets previously bypassed string validation and could reach the host
  as action arguments. Tests check all 256 escaped octets, preserving valid
  quoted-special and quoted-other values byte-for-byte.

- **`i;ascii-numeric` compares numbers of any length** (RFC 4790 §9.1.1). A
  run of digits past 18446744073709551615 did not fit in a uint64 and was
  treated as a non-number. So `18446744073709551616` equalled
  `18446744073709551617`, and any such number equalled `"cat"`. Digit runs are
  now compared exactly, with leading zeros ignored.

### Breaking

- **`fileinto` and `redirect` reject a repeated or unknown tagged argument**
  (RFC 5228 §2.4.2, RFC 3894 §3). `fileinto :copy :copy "x"`,
  `fileinto :bogus "x"` and `redirect :list "a@b"` were accepted and the tag
  ignored; each is now a parse error.

## [0.4.0] - 2026-10-19

### Added

- `Outcome.ExtendedReject` distinguishes an executed `ereject` (`true`) from
  `reject` (`false`), retaining `Disposition == Reject` and `RejectReason` for
  both. Hosts can apply the separate refusal policies in RFC 5429 §2.1 and §2.2.

- A separate RFC 5230 checklist maps vacation requirements to tests or explains
  the host responsibility. Vacation callbacks now retain `From`, `MIME`,
  `Handle`, `HandleSet`, and `SubjectSet`, so the host can generate responses
  and implement the specified response-tracking identity.

- `CHECKLIST.md` maps every requirement of RFC 5228, and each clause of RFC 5429 (`reject`, `ereject`), to the test that proves it, checked in CI by `internal/checklist`; a 95% statement-coverage gate (exceptions in `COVERAGE.md`) and fuzz targets `FuzzParse` and `FuzzDecodeEncodedCharacters`.

### Breaking

- **Vacation enforces RFC 5230 argument and safety rules.** A reason is required;
  duplicate/unknown tags and invalid `:from` mailbox-lists are errors. `:days`
  is clamped to 1..365 before duration conversion, preventing overflow.
  Replies require a personal recipient in the original addressing headers,
  matched against final envelope recipients plus `:addresses` aliases. List,
  automated, bulk and excluded system mail is suppressed. An absent envelope
  sender no longer falls back to the From header, and a second executed
  vacation action returns an evaluation error with the implicit keep.

- **`keep` with an argument and `reject`/`ereject` without a reason are refused
  at parse time.** RFC 5228 §4.3 gives `keep` no arguments, so `keep "x";` is now
  an error; the only argument accepted is `:flags` (RFC 5232 §5), and only once
  `imap4flags` is declared. RFC 5429 §2.1 and §2.2 give `reject` and `ereject`
  one required reason string, so a bare `reject;` is now an error. Scripts that
  relied on either are refused by `Parse` and `Validate`.

- **`Executor.Redirect` now receives the redirect's `:copy` tag:
  `Redirect(addr string, copy bool)`.** A host could not tell a bare `redirect`
  (forward only) from `redirect :copy` (forward and keep), and could only guess
  from a script-wide `require "copy"`, which says nothing about any one redirect.
  Every `Executor` implementation must add the parameter. Forward the message
  either way; whether it is also kept is still decided by
  `Outcome.ImplicitKeep` and any explicit `Keep`.

### Fixed

- `Parse` and `Validate` reject `reject`/`ereject` combined with `vacation` on
  the same control path, in either order, before any host callback (RFC 5429
  §2.4). Alternatives of one `if`/`elsif`/`else` remain valid; independent
  conditions are conservatively assumed able to match together.

- A string list with a trailing comma (`["a",]`) is rejected; RFC 5228 §8.1 puts a string after every comma.
- **`fileinto :copy` and `redirect :copy` no longer cancel the implicit keep.**
  RFC 3894 §3: a `:copy` action "does not cancel the implicit keep"; it files or
  forwards a copy in addition to whatever else happens to the message. Both
  cancelled it like their bare forms, so a script whose only action was
  `redirect :copy` forwarded the message and lost the local copy. Now
  `Outcome.ImplicitKeep` stays set after `:copy` actions alone. A `:copy` action
  never restores a keep already cancelled by `keep`, `discard`, or a bare
  `fileinto`/`redirect`, in either order.

## [0.3.0] - 2026-07-25

### Breaking

- **The evaluation `Outcome` now drives the RFC 5228 implicit keep, and hosts
  must honour it.** `Outcome` gains two fields, `ImplicitKeep bool` and
  `Error error`. On a `Continue` outcome a host MUST now deliver to the default
  mailbox when `Outcome.ImplicitKeep` is set, rather than relying on an explicit
  keep action being reported (RFC 5228 §2.10.2 implicit-keep model). This also
  fixes a mail-loss bug in which a `discard` combined with a delivering action
  dropped the deliveries the script had asked for. A runtime error during
  evaluation now fails safe to the implicit keep and is reported via
  `Outcome.Error` instead of losing the message (§2.10.6). Hosts that assumed
  "no delivering action reported means the script handled delivery" must be
  updated to check `Outcome.ImplicitKeep`.

- **`Vacation.Days int` is replaced by `Vacation.Interval time.Duration`.** The
  reply-suppression period is now carried as a duration so a `:seconds` interval
  (RFC 6131) keeps sub-day precision instead of collapsing to whole days by
  integer division. Callers must switch from reading a day count to reading the
  duration field.

### Added

- **`encoded-character` extension (`${hex:..}` / `${unicode:..}`).** A script
  that declares `require "encoded-character"` may now embed octets and Unicode
  code points inside string constants (RFC 5228 §2.4.2.4): `${hex:HH HH ...}` is
  replaced by the named octets and `${unicode:XXXX ...}` by the UTF-8 encoding
  of the named code points, so a key written as `${hex:24 24}` now matches
  `"$$"`. Previously the capability was rejected and the sequences were compared
  as their literal bytes. Decoding runs once, left to right, at parse time; a
  surrogate or out-of-range code point is an error, and a `${...}` run that does
  not match the grammar is left literal. Without the `require`, the sequences are
  untouched.

### Fixed

- **`vacation :seconds` keeps its precision, and a null reverse-path suppresses
  the auto-reply.** Two `vacation` bugs are fixed. A `:seconds` interval
  (RFC 6131) was converted to whole days by integer division, so any sub-day
  value collapsed to zero and the reply-suppression window was lost; the interval
  is now carried as a duration and honoured at second granularity, while `:days`
  stays day-granular. And a message with a null reverse-path (SMTP
  `MAIL FROM:<>`, which marks a bounce or auto-generated message) previously fell
  back to the `From` header for a reply target, so it still triggered an
  auto-reply and risked a mail loop; per RFC 5230 §4.6 and RFC 3834 no vacation
  response is now sent for such a message.

  As part of this, the `Vacation` struct's `Days int` field is replaced by
  `Interval time.Duration` (**breaking**): the Executor receives the full
  suppression period rather than a day count, and is only called when a reply is
  permitted (a null reverse-path or an otherwise-undeterminable target now
  suppresses the call instead of being handed to the host).

- **An unsupported comparator is rejected, and `i;ascii-numeric` no longer
  silently degrades to case-insensitive matching outside `:is`.** A comparator a
  script named with `:comparator` that the implementation does not support was
  quietly treated as the default `i;ascii-casemap`; RFC 5228 §2.7.3 requires an
  error instead, and one is now raised at parse time (only `i;octet`,
  `i;ascii-casemap`, and `i;ascii-numeric` are accepted). The numeric comparator
  `i;ascii-numeric` was honoured only by the `:is` match type and fell back to
  case-insensitive matching under `:contains`, `:matches`, and `:regex`. RFC 4790
  §9.1 gives `i;ascii-numeric` only equality and ordering, with no substring
  operation, so pairing it with those substring match types is now a parse error
  rather than a misleading text match. Its `:is` numeric equality — including the
  rule that any value not starting with a digit is positive infinity, so two
  non-numeric values are equal but a number never equals a non-number — is
  unchanged.

- **`discard` no longer terminates the script or drops mail the script asked to
  deliver, and the implicit keep is now modelled.** RFC 5228 §4.4 makes `discard`
  compatible with every other action — it only cancels the implicit keep — but
  the evaluator halted the whole script on `discard`, so `discard; fileinto "X";`
  filed nothing and the message was lost, and `fileinto "X"; discard;` recorded
  the `fileinto` yet reported `Discard`, so the host dropped it. The evaluator now
  accumulates the actions a script selects and resolves delivery only after the
  script finishes (or a `stop`): `discard` cancels the implicit keep but lets the
  rest of the script run, and a delivering action always wins over it
  (`fileinto`+`discard` is equivalent to `fileinto`). A bare `discard` with no
  delivering action still drops the message.

  Alongside it, `Outcome` gains the RFC 5228 §2.10.2 implicit keep: unless a
  `keep`, `fileinto`, `redirect`, or `discard` cancels it, `Outcome.ImplicitKeep`
  is set and the host must deliver to the default mailbox (previously a host had
  no way to tell "keep to the default mailbox" apart from "the script handled
  delivery"). A runtime error during evaluation now fails safe to that implicit
  keep (§2.10.6), reported via the new `Outcome.Error`, rather than losing the
  message. A `stop` before any action still delivers via the implicit keep.

- **Duplicate delivery actions no longer deliver to the same destination
  twice.** Each `keep`, `fileinto`, and `redirect` was passed straight to the
  `Executor`, so a script naming the same destination more than once — two
  `fileinto "INBOX.X"`, two `redirect a@b`, or repeated `keep` — caused two
  deliveries there. RFC 5228 §2.10.3 requires that a message not be delivered
  to the same destination more than once regardless of how many actions name
  it. The evaluator now records the destinations already dispatched and
  collapses a repeat to a single Executor call; the first occurrence executes,
  preserving script order and interleaving with other actions, and distinct
  destinations still each execute. A `:copy` action (RFC 3894) targets a
  destination distinct from a consuming one, so it is not collapsed with it.

- **An empty `[]` string list is now a parse error.** RFC 5228 §8.2 defines
  `string-list` as `"[" string *("," string) "]" / string`, so a bracketed list
  must contain at least one string; there is no production for an empty `[]`.
  The parser accepted it anyway in every string-list position, which let
  `exists []` evaluate true vacuously (the "all headers present" loop had
  nothing to falsify) and silently accepted meaningless lists such as
  `require []` or `header :is [] "x"`. An empty `[]` is now rejected at parse
  time wherever a string list is expected.

- **`envelope :is "from" ""` now matches a null reverse-path.** A null
  reverse-path (SMTP `MAIL FROM:<>`, i.e. a bounce) has an empty-string
  envelope `from` value (RFC 5228 §5.4), but it was treated as absent, so the
  canonical bounce-detection idiom `if envelope :is "from" ""` never matched
  and `:localpart`/`:domain` on it misbehaved. `Envelope` gains a `FromNull`
  field: set it (with `From` left empty) to represent a null reverse-path, which
  now produces a single empty-string value that matches `""`. A genuinely
  absent sender (empty `From`, `FromNull` unset) still produces no value.

- **`address` test now parses non-structured headers as RFC 5322 addresses.**
  When the `address` test was applied to a header outside the structured set
  (From/To/Cc/Bcc) — e.g. `Resent-From` — the raw header value was split on the
  last `@` instead of being parsed. This produced wrong `:localpart`/`:domain`
  results (for `Resent-From: "a@b" <c@d>`, `:domain` yielded `d>`), matched
  display-name and comment text, and let unparseable values match
  `:localpart`/`:domain`. Such headers are now parsed with
  `net/mail.ParseAddressList` and evaluated on the parsed addr-spec; values that
  do not parse as an address are skipped (RFC 5228 §5.1, §2.7.4).

- **`header` and `exists` tests now unfold and trim header values.** Header
  values were compared verbatim, so a value with surrounding whitespace or a
  folded continuation line failed an otherwise-correct match — `header :is
  "X-Test" "foo"` was false for `X-Test:   foo`, and a value folded across
  lines never matched its logical text. Values are now unfolded (RFC 5322
  §2.2.3, folding whitespace collapsed to a single space) and stripped of
  leading and trailing whitespace before matching, as RFC 5228 §5.7 requires.

- **Duplicate or inapplicable tagged arguments on a test are now rejected.** A
  repeated match type, a repeated comparator, a repeated or misplaced address
  part, a duplicate body transform, or any tagged argument not defined for the
  test (or for `size`) was previously accepted last-wins or silently consumed,
  masking script mistakes. These are now parse errors, and supplying both
  `:over` and `:under` to `size` is rejected (RFC 5228 §2.7.1, §2.7.3, §2.7.4,
  §5.7, §5.9).

- **A nil message no longer panics, and body traversal is bounded.** Evaluating
  a test against a nil message dereferenced it and panicked; evaluation now
  guards the entry point and takes the implicit keep instead. The body test and
  the size reconstruction walked the MIME part tree with no limit, so a deeply
  nested or part-heavy structure supplied by the host could exhaust the goroutine
  stack or burn unbounded CPU; both walks are now bounded by nesting depth and by
  total parts visited, beyond which the deeper or remaining parts are treated as
  absent.

- **`:matches "?"` matches exactly one octet, not one decoded character.** The
  `?` wildcard was compiled to a regexp in which `.` spans a whole UTF-8 rune, so
  a two-octet character counted as a single position — `?` wrongly matched it and
  `??` wrongly failed. This contradicts the octet-string model of the `i;octet`
  and `i;ascii-casemap` comparators (RFC 5228 §2.7.1). Matching is now a direct
  byte-level glob: `?` matches one octet, `*` scans octet by octet, and a
  backslash escapes the following octet; `*` semantics and literal matching are
  unchanged.

- **`i;ascii-casemap` folds case only within US-ASCII.** The default comparator
  used Go's Unicode-aware case folding, so distinct Unicode characters compared
  equal to ASCII letters — long s (U+017F) folded to `s`, the Kelvin sign
  (U+212A) to `k` — letting a filter keyed on ASCII text be evaded with
  look-alikes or over-match. Case is now folded only for A–Z ⇄ a–z; every octet
  ≥ 0x80 and every non-letter ASCII octet is compared byte-exact (RFC 4790 §9.2,
  RFC 5228 §2.7.3). `i;octet` and `i;ascii-numeric` are unchanged.

- **The `size` test counts the whole message, including header octets.** `size`
  summed only the body and attachment octets, so the header block and the blank
  separator line were excluded and `:over`/`:under` thresholds were wrong for any
  real message. The size is now the entire message as it appears on the wire
  (RFC 5228 §5.9). `Message` gains a `RawSize` field a host can set to supply the
  exact wire size, which is then used verbatim; otherwise the size is
  reconstructed from the serialized header block, the CRLF separator, and the
  body.

- **Quoted strings reject a bare CR and NUL and accept a CRLF line break.** The
  quoted-string lexer treated a lone LF as an error but copied a bare CR and a
  NUL octet straight into the value, while rejecting a valid CRLF line break as
  an unterminated string — inverting RFC 5228 §8.1. A CRLF pair inside a quoted
  string is now accepted (and counted for line numbering), and a bare CR, a lone
  LF, or a NUL octet is a lex error reported at the string's start line.

## [0.2.0] - 2026-07-25

### Breaking

- **`require` is now enforced (RFC 5228 §2.10.5, §3.2).** Earlier releases
  recorded `require` declarations but never checked them, so a script could use
  an extension it had not declared and still run. Enforcement is now applied in
  both directions at parse time, which means scripts that were previously
  accepted may now be rejected. **Audit your scripts and add a
  `require ["extension", ...];` declaration for every extension they use.**
  Specifically:
  - Using an extension command (`fileinto`, `reject`/`ereject`, `imap4flags`,
    `vacation`, `notify`), test (`envelope`, `body`), match type (`:regex`),
    comparator (`i;ascii-numeric`), or tagged argument (`:create`, `:copy`,
    `:flags`) without declaring it via `require` is now an error.
  - `require` of an extension this package does not implement is now an error.
  - `require` must precede every other command and may not appear inside a
    block.
  - An unknown command or test (including a typo) is now a parse error rather
    than a silently ignored no-op.

### Security

- Bound parser recursion depth to prevent a stack-overflow denial of service
  from scripts with deeply nested tests (`not`/`allof`/`anyof`) or control
  blocks (`if`/`elsif`/`else`). Nesting is capped at parse time via the new
  `DefaultMaxDepth` (64), overridable with `WithMaxDepth` (RFC 5228 §2.10.7).
- Reject control characters (CR, LF, NUL, ...) in a `redirect` target address.
  A `text:` multi-line literal could previously smuggle these bytes into the
  address, exposing hosts that build SMTP commands or headers from the target
  to command/header injection (RFC 5228 §2.10.6).
- Reject `K`/`M`/`G` size-quantifier overflow instead of silently wrapping
  `int64`. A wrapped (negative) limit had inverted `size :over` / `size :under`
  filters; an unrepresentable quantified value is now a parse error
  (RFC 5228 §2.4.1).

## [0.1.1] - 2026-07-23

- Documentation and packaging polish for the initial public release.

## [0.1.0] - 2026-07-23

- Initial public release: RFC 5228 Sieve parser and evaluator.
