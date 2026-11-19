# Changelog fragments

A pull request that changes behaviour adds one fragment file here and does not
edit `CHANGELOG.md`. Two pull requests then never conflict over the same
release section. The release step assembles the fragments into `CHANGELOG.md`.

## Writing a fragment

- One file per change, named `<area>-<slug>.md` in lower case, for example
  `sieve-variables.md` or `grammar-nul-octets.md`.
- The file holds a single line, ready for the release notes:
  `<Heading>: <text>`.
- `<Heading>` is one of `Added`, `Changed`, `Changed (breaking)`, `Fixed` or
  `Known gaps`, the headings `CHANGELOG.md` already uses. The heading is not
  part of the printed note; it picks the group the note goes into.
- Write the note as the other entries in `CHANGELOG.md` read, and name the
  issue where there is one.
- No file is edited by more than one pull request. A later change to the same
  feature adds its own fragment.

For example, `changelog.d/sieve-variables.md`:

```
Added: The variables extension (RFC 5229, #97): `set` with its six modifiers and the `string` test.
```

## Building the notes

Print the notes for the fragments now waiting, sorted by file name and grouped
under their headings:

```sh
go run ./internal/changelog
```

Cut a release. This inserts a `## [<version>] - <date>` section below
`## [Unreleased]` in `CHANGELOG.md`, writes the notes into it and deletes the
fragment files it used (this file stays):

```sh
go run ./internal/changelog --release 0.9.0 2026-11-20
```

It refuses to run when the version already has a section in `CHANGELOG.md`,
when a fragment is malformed or when no fragment is waiting. Text written by
hand under `## [Unreleased]` is left where it is; move it into the new section
before tagging.
