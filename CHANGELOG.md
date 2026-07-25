# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
While the project is pre-1.0, a breaking change bumps the minor version.

## [Unreleased]

## [0.3.0] - 2026-07-25

### BREAKING CHANGES

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

## v0.2.0

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

## v0.1.1

- Documentation and packaging polish for the initial public release.

## v0.1.0

- Initial public release: RFC 5228 Sieve parser and evaluator.
