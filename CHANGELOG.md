# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
While the project is pre-1.0, a breaking change bumps the minor version.

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
