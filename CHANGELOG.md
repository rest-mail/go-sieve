# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
While the project is pre-1.0, a breaking change bumps the minor version.

## Unreleased

### Fixed

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
