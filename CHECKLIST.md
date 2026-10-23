# RFC 5228 checklist


Every requirement [RFC 5228](https://www.rfc-editor.org/rfc/rfc5228) (the Sieve
language) places on an implementation of the language, item by item, with the
tests that cover it. The list takes in the MUSTs, the normative behaviour that
carries no keyword (level "Normative"), such as the grammar and the semantics of
each test and action, and the MAYs the library implements. A section number
alone is RFC 5228.

An item that does not apply to a library (because it is the host's job, say) or
is a known gap says "n/a" and why. CI checks this file against the tests with
`go run ./internal/checklist`; the format is described in the
[README](README.md#the-rfc-checklist).

RFC 5429 (`reject` and `ereject`), RFC 5230 (`vacation`), RFC 3894 (`copy`),
RFC 5232 (`imap4flags`), RFC 5173 (`body`), RFC 5490 (`mailbox`) and
RFC 6131 (`vacation-seconds`) are checked clause by clause in their own sections. The `Received` and loop requirements of
`redirect` belong to the host, as recorded below.

`TestDifferentialEvaluate` and `TestDifferentialParse` check the RFC 5228 items
they name against an independent implementation,
[foxcpp/go-sieve](https://github.com/foxcpp/go-sieve), used only by the tests.


## RFC 5228 core audit (2026-10-22)

Audited sections **1–14 and every subsection**, comparing explicit MUST,
MUST NOT, SHOULD, SHOULD NOT and RECOMMENDED clauses and their surrounding
text against the existing rows. Sections 1/1.1/1.2 provide context and examples;
2.1–2.10 cover syntax, matching and execution; 3.1–3.3 controls;
4.1–4.4 actions; 5.1–5.10 tests; 6/6.1–6.3 extension and registration duties;
7 transport; 8.1–8.3 grammar; 9 the extended example; 10 security;
11–13 acknowledgments/references; and 14 the change summary. Sections 11–14
add no separate executable MUST/SHOULD obligation. Section 9 was reviewed as
an example, not certified as an exhaustive interoperability vector.

The new core rows below supplement, rather than replace, the earlier tables.
This is a requirements inventory, not proof of complete RFC conformance.
In particular, **n/a marked as a known gap is not an exemption from the RFC**.
Host duties need integration tests in the consuming MTA; extension-author and
registry duties are specification work. Imported RFCs were not freshly audited.
The extension audits and the owner decisions on hasflag, delivered flags,
`:raw` and mailboxexists remain unchanged.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-CORE-01 | 2.4.1 | MUST | Support every integer from zero through 2147483647 inclusive. | `TestRFC5228MinimumIntegerRange` |
| SIEVE-CORE-02 | 2.4.2, 8.1 | MUST | Accept all grammar-valid string octets, including non-UTF-8 bytes, rather than requiring Unicode text. | `TestRFC5228OpaqueStringOctets`, `TestRFC5228SourceNonNULOctets` |
| SIEVE-CORE-03 | 2.4.2.2 | MUST NOT | Invalid header-name operands cause no error and match no field, including with header, address and exists tests. | `TestRFC5228InvalidHeaderNamesNeverMatch` |
| SIEVE-CORE-04 | 2.4.2.2, 2.7.2 | SHOULD | Interpret MIME encoded header text as Unicode encoded in UTF-8 before comparing. | n/a: host adaptation currently owns MIME header decoding; Headers.Raw is unfolded but not RFC 2047 decoded by this library. Passing undecoded words is a known integration gap. |
| SIEVE-CORE-05 | 2.7.2 | MUST | Within the recommended conversion behavior, support US-ASCII, ISO-8859-1, the ASCII subset of ISO-8859-* and UTF-8. | n/a: the host must supply decoded UTF-8 header values. The library has no charset decoder; callers must verify these minimum conversions in their adapter. |
| SIEVE-CORE-06 | 2.7.2 | MUST | If the recommended charset behavior is unsupported, never consider strings equal when either contains octets above 127. | n/a: known gap for hosts supplying undecoded values: the library neither enforces charset conversion nor this restrictive alternative; its comparators permit equal non-ASCII octets. A conforming integration must provide the conversion path in CORE-04/05. |
| SIEVE-CORE-07 | 2.7.2 | SHOULD NOT | An encoded NUL must not terminate the header content being compared. | `TestRFC5228EncodedNULDoesNotTruncate` |
| SIEVE-CORE-08 | 2.6.2 | SHOULD NOT | Tagged arguments should not themselves take tagged arguments as values. | n/a: extension syntax design duty; the implemented tags take literal values or no value, not nested tags. |
| SIEVE-CORE-09 | 2.10.3 | MUST NOT | A script requesting delivery to a mailbox twice is not an error. | `TestRFC5228RepeatedDeliveryIsNotError` |
| SIEVE-CORE-10 | 2.10.4 | MUST | Allow at least one keep or fileinto; implementations without fileinto must allow keep. | `TestRFC5228RepeatedDeliveryIsNotError` |
| SIEVE-CORE-11 | 2.10.5, 2.10.6 | MUST | Check executed code syntactically, semantically and at runtime and do not execute code after an unavailable require. | `TestRFC5228ChecksExecutedCode`, `TestRFC5228RuntimeChecksStopProcessing` |
| SIEVE-CORE-12 | 2.10.6 | MUST | Notify the user of errors and which actions, if any, were taken. | n/a: the host must notify the user using Parse errors or Outcome.Error and its own callback/action journal. Outcome alone does not enumerate already completed or partially failed host actions. |
| SIEVE-CORE-13 | 2.10.7 | MUST | Support fifteen nested blocks with the default parser configuration. | `TestRFC5228FifteenNestingLevels` |
| SIEVE-CORE-14 | 2.10.7 | MUST | Support fifteen nested test lists with the default parser configuration. | `TestRFC5228FifteenNestingLevels` |
| SIEVE-CORE-15 | 4, 5 | MUST | Support keep, discard, redirect and address, allof, anyof, exists, false, header, not, size and true. | `TestRFC5228MandatoryCoreSupport` |
| SIEVE-CORE-16 | 4.1 | SHOULD | Reencode UTF-8 mailbox names when the mailstore uses a different encoding. | n/a: Executor.FileInto receives the script string; the host owns mailstore encoding, including modified UTF-7 for stores that require it. |
| SIEVE-CORE-17 | 4.2 | MUST | Preserve existing Received fields and increase their count before a redirected message leaves the implementation. | n/a: the host owns wire serialization and trace fields; a Received added before filtering can satisfy the increase. No message header is added by this evaluator. |
| SIEVE-CORE-18 | 4.2 | MUST | A redirected message with a null incoming envelope sender must retain a null outgoing envelope sender. | n/a: Executor.Redirect receives the destination and copy flag; the host retains the original envelope and selects the outgoing reverse-path. This cannot be certified by a destination callback test. |
| SIEVE-CORE-19 | 4.2, 10 | MUST | Provide administrator controls to limit redirect abuse, specifically the number of redirects per script. | n/a: host policy duty; there is no built-in redirect count option. A host Executor must enforce the limit and signal a runtime error when exceeded. |
| SIEVE-CORE-20 | 4.2 | MUST NOT | A redirect silently ignored for policy reasons must not cancel the implicit keep. | n/a: known API gap: Redirect has no return value for a policy skip and the evaluator cancels keep before the callback. A host must not silently skip and rely on Outcome.ImplicitKeep; it must preserve delivery itself or signal an error for the fail-safe keep. |
| SIEVE-CORE-21 | 4.4 | MUST | Discard is silent and does not generate a DSN, MDN or other non-delivery notification. | `TestRFC5228DiscardIsSilent` |
| SIEVE-CORE-22 | 5.1 | MUST | Restrict address tests to address-bearing fields and include From, To, Cc, Bcc, Sender, Resent-From and Resent-To. | `TestRFC5228AddressHeaderRestriction` |
| SIEVE-CORE-23 | 5.4 | MUST | Drop source routes before every envelope comparison, including all, localpart and domain. | `TestRFC5228EnvelopeDropsSourceRoutes` |
| SIEVE-CORE-24 | 5.4 | SHOULD | Consider unknown envelope parts an error. | `TestRFC5228_EnvelopeUnknownPartsRejected` |
| SIEVE-CORE-25 | 6 | Normative | Capability strings are case-sensitive even though command identifiers are case-insensitive. | `TestRFC5228CapabilityStringsCaseSensitive` |
| SIEVE-CORE-26 | 6 | MUST | An extension defines a unique capability string and uses a different name when a new version changes functionality. | n/a: extension specification/registration duty; capability recognition is inventoried by EXT-CAP-01, not proof of registration or future version naming. |
| SIEVE-CORE-27 | 2.10.1, 2.10.5, 6 | MUST | Extension specifications state action compatibility and implicit-keep interactions and must not change require or its argument interpretation. | n/a: extension-author duty. Implemented extension behavior has its own audited sections below; the parser keeps core require semantics. |
| SIEVE-CORE-28 | 6 | MUST | A registered capability prefix ends with a hyphen and does not overlap an existing registration. | n/a: IANA/extension-registration duty; this package does not allocate capability prefixes. |
| SIEVE-CORE-29 | 6 | MUST | Extensions generating mail or protocol requests consider loop suppression, at least in security documentation. | n/a: extension specification duty; host transport/suppression requirements are recorded in the vacation, reject and redirect rows. Local notify is not a claim of a standardized notification extension. |
| SIEVE-CORE-30 | 6.1 | SHOULD | A vnd. extension name is followed by the vendor and product name. | n/a: naming guidance for vendor extension authors; no vnd. capability is implemented. |
| SIEVE-CORE-31 | 6.2 | SHOULD | Extensions intended for interoperable use use IETF-controlled capability names. | n/a: extension specification/registration duty; local regex and notify are explicitly not claims of standards conformance. |
| SIEVE-CORE-32 | 10 | SHOULD | Protect stored scripts against unauthorized retrieval. | n/a: the host owns storage, authentication and script retrieval; this library only parses supplied text. |
| SIEVE-CORE-33 | 10 | SHOULD | Set the redirect count limit to one when there is no use case for multiple destinations. | n/a: administrator policy decision in the host; the library does not know deployment use cases or impose a destination count. |
| SIEVE-CORE-34 | 10 | MUST | Provide facilities to log redirect use to investigate abuse. | n/a: Executor.Redirect is the host logging boundary; durable logs, identity and delivery outcome are host responsibilities, not provided by the evaluator. |
| SIEVE-CORE-35 | 2.4.2, 8.1 | SHOULD NOT | Script authors should not backslash-escape characters other than quote and backslash without an extension defining them. | n/a: script-author/generator guidance; parser acceptance of undefined escapes is required and covered by SIEVE-LEX-03. |
| SIEVE-CORE-36 | 2.4.2 | SHOULD | Script authors should properly dot-stuff multiline strings to avoid ambiguous single-dot-prefixed lines. | n/a: script-author/generator guidance. The parser must still preserve a singly prefixed dot as proved by CORE-38. |
| SIEVE-CORE-37 | 2.1, 8.1 | Normative | Raw NUL is forbidden anywhere in script source, including comments and multiline strings; encoded-character may produce NUL values. | `TestRFC5228SourceNULRejected`, `TestRFC5228SourceNULDiagnostic`, `TestRFC5228SourceNonNULOctets`, `FuzzParse` |
| SIEVE-CORE-38 | 2.4.2, 8.1 | Normative | Multiline string values preserve CRLF and remove a leading dot only when it is doubled; a single dot followed by other text remains literal. | `TestRFC5228MultilinePreservesSingleDotAndCRLF`, `TestRFC5228SourceCRLFPreserved` |
| SIEVE-CORE-39 | 2.4.2.3 | Normative | A redirect target must be a single mailbox, excluding source routes and groups; validate after encoded-character decoding. | `TestRFC5228RedirectAddressSyntax`, `TestRFC5228RedirectAddressPreserved`, `TestRFC5228RedirectDecodedAddress` |
| SIEVE-CORE-40 | 2.7.4 | Normative | Invalid addresses must not match localpart or domain tests. | n/a: raw headers are parsed and covered by SIEVE-TST-02, but structured Address and Envelope values are trusted without complete address validation. The host must validate those inputs; arbitrary malformed structured values remain a known gap. |
| SIEVE-CORE-41 | 5.1 | SHOULD | Include other fields using an address-list body in address tests. | `TestRFC5228AddressHeaderRestriction` |
| SIEVE-CORE-42 | 5.4 | Normative | Only envelope recipient information for the evaluated user is available in the to part. | n/a: the host must populate Envelope.To with only the current delivery recipient identities; the library cannot infer user ownership from an arbitrary supplied list. |
| SIEVE-CORE-43 | 7 | Normative | Use application/sieve when transmitting a Sieve script as a MIME entity. | n/a: this parser has no script transmission or MIME serialization facility. |
| SIEVE-CORE-44 | 2.1–2.3, 8.1 | Normative | CR and LF occur only as the CRLF line ending in script source. | n/a: known lexer gap outside quoted strings: bare LF is still accepted in whitespace, comments and multiline strings. SIEVE-CORE-49 proves whole-source bare-CR rejection; SIEVE-LEX-03 proves quoted-string validation. |
| SIEVE-CORE-45 | 2.4.2.3 | Normative | Accept the complete sieve-address grammar, including obsolete addr-spec forms and folding whitespace in domain literals inherited from RFC 2822. | n/a: redirect uses Go net/mail mailbox syntax, which rejects some obsolete forms. Domain literals are checked as non-empty dtext independently of the Go version (IPv4, IPv6: and general literals are tested by CORE-39), but FWS inside a literal is rejected; this is not complete grammar conformance. |
| SIEVE-CORE-46 | 2.4.2.2, 5.1, 5.5, 5.7; RFC 5322 §3.6.8 | Normative | Header field names use ASCII case-insensitive matching, independently of the value comparator; non-ASCII names in raw message headers cannot alias valid names. | `TestRFC5228RawHeaderNameASCIICase`, `TestRFC5228RawHeaderNameOctets` |
| SIEVE-CORE-47 | 5.7; RFC 5322 §2.2.3 | Normative | Before header matching, unfold by removing only CRLF immediately followed by SP or HTAB; preserve the whitespace octets rather than collapsing them. | `TestRFC5228HeaderUnfoldPreservesWhitespace`, `TestRFC5228HeaderUnfoldWSPSequences` |
| SIEVE-CORE-48 | 5.7; RFC 5322 §§2.2.3, 3.2.2 | Normative | Header comparison strips mail whitespace at value boundaries, preserving other control octets and Unicode spaces as content. | `TestRFC5228HeaderTrimPreservesNonWSP`, `TestRFC5228HeaderTrimBoundaryOctets` |
| SIEVE-CORE-49 | 2.1–2.3, 8.1 | Normative | A raw CR must be followed immediately by LF everywhere in script source, including comments and multiline strings; encoded-character may produce a bare CR value. | `TestRFC5228SourceBareCRRejected`, `TestRFC5228SourceCRFollowingOctets`, `TestRFC5228SourceCRDiagnostic`, `TestRFC5228SourceCRLFPreserved`, `FuzzParse` |

## Lexical structure (§2.2–2.4)

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-LEX-01 | 2.2 | Normative | Whitespace separates tokens; comments are `#` to the end of the line or bracketed `/* */`, and a bracketed comment must be closed. | `TestSieve_Comments`, `TestRFC5228_SyntaxErrors`, `FuzzParse`, `TestLexer_Errors`, `TestDifferentialParse` |
| SIEVE-LEX-02 | 2.1 | Normative | Command, test and tag names are case-insensitive. | `TestRFC5228_IdentifiersCaseInsensitive`, `FuzzParse` |
| SIEVE-LEX-03 | 2.4.2, 8.1 | Normative | A quoted string ends at the next unescaped `"`; `\"` and `\\` stand for `"` and `\`; NUL and bare CR/LF are forbidden; no escape permits NUL, CR or LF; an unescaped CRLF is allowed. | `TestSieve_EscapedQuoteInString`, `TestLexQuotedString_RejectsBareCRAndNUL`, `TestLexQuotedString_AcceptsCRLF`, `TestQuotedString_EscapedOctets`, `TestQuotedString_EscapedLineBreakAndTruncation`, `FuzzParse`, `TestLexer_Errors`, `TestDifferentialParse` |
| SIEVE-LEX-04 | 2.4.2 | Normative | A tagged argument is a colon and an identifier; a test or command takes each applicable tag at most once, and a tag it does not define is an error. | `TestValidateSieve_RejectsDuplicateOrInapplicableTags`, `TestValidateSieve_AcceptsSingleTaggedArgs`, `TestSieve_UnknownCommandRejected` |
| SIEVE-LEX-05 | 2.4.2, 8.1 | Normative | A multi-line string is `text:`, optional spaces or tabs, and a hash comment or a line break, then lines up to a line holding only `.`; a leading `.` on a line is doubled (dot-stuffing) and the doubling is removed; an unterminated one is an error. | `TestSieve_MultiLineString`, `TestSieve_MultiLineDotStuffing`, `TestRFC5228_SyntaxErrors`, `FuzzParse`, `TestLexer_Errors`, `TestMultiLineString_HashCommentAfterMarker` |
| SIEVE-LEX-06 | 2.4.2.4 | Normative | With `require "encoded-character"`, `${hex:…}` and `${unicode:…}` in a string stand for the octets or code points they name; text that does not fit the grammar stays as written, a surrogate or code point above U+10FFFF is an error, and the result is not scanned again. | `TestEncodedCharacter_HexDecodesInStringConstant`, `TestEncodedCharacter_HexKeyMatchesHeader`, `TestEncodedCharacter_UnicodeDecodesToUTF8`, `TestEncodedCharacter_MalformedLeftLiteral`, `TestEncodedCharacter_NotRequiredStaysLiteral`, `TestDecodeEncodedCharacters_RFCExamples`, `TestDecodeEncodedCharacters_OutOfRangeUnicode`, `TestEncodedCharacter_InvalidUnicodeIsError`, `FuzzDecodeEncodedCharacters`, `TestEncodedCharacter_SequenceErrors`, `TestEncodedCharacter_EnvelopeStringLists`, `TestEncodedCharacterUnicodeMalformedRangeLiteral`, `TestEncodedCharacterUnicodeMalformedRangeOctets`, `TestEncodedCharacterBlankGrammar` |
| SIEVE-LEX-07 | 2.4.2.1 | Normative | A string list is one string or `[` strings separated by commas `]`; it holds at least one string, and a comma is followed by a string. | `TestSieve_KeyList`, `TestSieve_HeaderList`, `TestValidateSieve_RejectsEmptyStringList`, `TestRFC5228_SyntaxErrors`, `FuzzParse`, `TestDifferentialParse` |
| SIEVE-LEX-08 | 2.4.1 | Normative | A number is digits with an optional quantifier K, M or G (powers of 1024, in either case); a number the implementation cannot represent is an error. | `TestSieve_Size_Quantifier`, `TestValidateSieve_RejectsSizeOverflow`, `TestValidateSieve_AcceptsRepresentableSizes`, `TestRFC5228_SizeTest`, `FuzzParse`, `TestLexer_Errors` |
| SIEVE-LEX-09 | 2.8 | Normative | A block is commands inside `{` and `}`, and may be empty or nested. | `TestSieve_IfElsifElse`, `TestRFC5228_SyntaxErrors`, `TestDifferentialParse` |
| SIEVE-LEX-10 | 2.9 | Normative | A command ends with `;` or, for a control command, with a block; a missing `;`, a missing block and an unbalanced brace are errors. | `TestRFC5228_SyntaxErrors`, `TestValidateSieve_Valid`, `TestValidateSieve_AcceptsNewConstructs`, `FuzzParse`, `TestDifferentialParse`, `TestParse_TopLevelLeftoverTokens` |
| SIEVE-LEX-11 | 2.1, 8.2 | Normative | A command takes exactly the arguments its grammar gives it: `stop`, `discard` and the tests take no extra arguments, and a test list is not empty. | `TestRFC5228_SyntaxErrors`, `TestParse_NestedErrorsPropagate` |
| SIEVE-LEX-12 | 2.4.2.4 | MUST NOT | Raise an out-of-range Unicode error unless the complete encoded-character sequence is well-formed according to the grammar. | `TestEncodedCharacterUnicodeMalformedRangeLiteral`, `TestEncodedCharacterUnicodeMalformedRangeOctets`, `TestEncodedCharacterBlankGrammar` |

## Control commands (§3)

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-CMD-01 | 2.10.5 | MUST | A command or test that is not defined, and not declared with `require`, is an error. | `TestSieve_UnknownCommandRejected`, `TestSieve_UnknownTestRejected`, `TestRequire_RejectsUnknownCommandsAndTests`, `TestRequire_RejectsUndeclaredExtensionUse`, `TestParseSieveScript_RequiresAndCommands`, `TestValidateSieve_Errors`, `TestEvalTest_UnknownIsFalse`, `TestDifferentialParse` |
| SIEVE-CMD-02 | 3.1 | Normative | `if` runs its block when its test is true; `elsif` and `else` follow only an `if` (or `elsif`), at most one `else`, and only the first branch whose test is true runs. | `TestSieve_IfElsifElse`, `TestSieve_ElsifOnlyFirstMatchRuns`, `TestRFC5228_SyntaxErrors`, `FuzzParse`, `TestDifferentialEvaluate`, `TestDifferentialParse` |
| SIEVE-CMD-03 | 3.2 | MUST | `require` comes before every other command; a capability the implementation does not support is an error; an extension is only usable once declared. Several `require` commands may open a script. | `TestRequire_RejectsUnsupportedExtension`, `TestRequire_RejectsUndeclaredExtensionUse`, `TestRequire_RejectsMisplacedRequire`, `TestRequire_AcceptsProperlyDeclaredScripts`, `TestRequire_ErrorMentionsCapability`, `TestRFC5228_MultipleRequires`, `FuzzParse`, `TestDifferentialParse` |
| SIEVE-CMD-04 | 3.3 | Normative | `stop` ends the script, from any depth of nesting; actions already requested still happen and the implicit keep applies as if the script had ended. | `TestSieve_Stop`, `TestSieve_TopLevelStop`, `TestSieve_StopBeforeActionKeepsImplicitly`, `TestSieve_DiscardThenStopDrops`, `TestRFC5228_StopInNestedBlock`, `TestDifferentialEvaluate`, `TestDifferentialStop` |
| SIEVE-CMD-05 | 10 | MAY | Limit how deeply blocks and test lists nest. | `TestParse_RejectsDeeplyNestedTests`, `TestParse_RejectsDeeplyNestedBlocks`, `TestParse_AcceptsNestingUpToLimit`, `TestParse_WithMaxDepth`, `TestParse_DeeplyNestedDoesNotCrash` |

## Tests (§5)

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-TST-01 | 5.1 | Normative | `address` tests the addresses in the named address headers, with an address part (default `:all`), a comparator and a match type. | `TestSieve_AddressLocalpart`, `TestSieve_AddressDomain`, `TestSieve_AddressAllDefault`, `TestSieve_AddressCc`, `TestSieve_AddressRawHeader_DisplayName`, `TestSieve_AddressRawHeader_AngleAddr`, `TestHeaderTest_StructuredHeaders`, `TestAddress_RawHeaderOnlyNamed`, `TestDifferentialEvaluate`, `TestRFC5228RawHeaderNameASCIICase` |
| SIEVE-TST-02 | 5.1 | Normative | A header value in the named headers that does not parse as an address list contributes no address. | `TestSieve_AddressRawHeader_Unparseable` |
| SIEVE-TST-03 | 5.2 | Normative | `allof` is true when every test in its list is true. | `TestSieve_Allof`, `TestSieve_Allof_OneFalse`, `TestRFC5228_AllofAnyofNested`, `TestRFC5228_SyntaxErrors`, `TestDifferentialEvaluate` |
| SIEVE-TST-04 | 5.3 | Normative | `anyof` is true when any test in its list is true. | `TestSieve_Anyof`, `TestSieve_Anyof_NoneTrue`, `TestRFC5228_AllofAnyofNested`, `TestDifferentialEvaluate` |
| SIEVE-TST-05 | 5.4 | Normative | With `require "envelope"`, `envelope` tests the `from` or `to` part of the SMTP envelope (and any other part the implementation supports) with an address part, comparator and match type; the null reverse-path is the empty string. | `TestSieve_EnvelopeDomainPart`, `TestSieve_EnvelopeFrom_Is`, `TestSieve_EnvelopeTo_Is`, `TestSieve_EnvelopeFrom_Contains`, `TestSieve_EnvelopeFrom_NoMatch`, `TestSieve_EnvelopeFrom_NullReversePath`, `TestSieve_EnvelopeFrom_AbsentDoesNotMatchEmpty`, `TestSieve_EnvelopeFrom_NullReversePath_NoFalseMatch`, `TestSieve_EnvelopeFrom_NullReversePath_Parts`, `TestSieve_EnvelopeFrom_Resolved`, `TestSieve_EnvelopeTo_Resolved`, `TestRFC5228_EnvelopePartsCaseInsensitive`, `TestEncodedCharacter_EnvelopeStringLists`, `TestDifferentialEvaluate` |
| SIEVE-TST-06 | 5.5 | Normative | `exists` is true when every named header is present. | `TestSieve_Exists`, `TestSieve_Exists_AllRequired`, `TestSieve_Exists_RawHeader`, `TestRFC5228_AbsentHeaderNeverMatches`, `TestRFC5228_HeaderNamesCaseInsensitive`, `TestDifferentialEvaluate`, `TestRFC5228RawHeaderNameASCIICase` |
| SIEVE-TST-07 | 5.6, 5.10 | Normative | `false` is never true and `true` is always true. | `TestSieve_TrueFalse`, `TestDifferentialEvaluate` |
| SIEVE-TST-08 | 5.7 | Normative | `header` is true when any value of any named header matches any key; header names match without regard to case, a header the message lacks matches nothing (not even the empty string), and folded values are unfolded with leading and trailing white space removed. | `TestSieve_HeaderContains`, `TestSieve_HeaderTrimsWhitespace`, `TestSieve_HeaderTrimsWhitespaceContains`, `TestSieve_HeaderUnfoldsContinuation`, `TestRFC5228HeaderUnfoldPreservesWhitespace`, `TestRFC5228HeaderUnfoldWSPSequences`, `TestSieve_HeaderList`, `TestSieve_KeyList`, `TestRFC5228_AbsentHeaderNeverMatches`, `TestRFC5228_HeaderNamesCaseInsensitive`, `TestRFC5228_SyntaxErrors`, `TestHeaderTest_StructuredHeaders`, `TestDifferentialEvaluate`, `TestRFC5228RawHeaderNameASCIICase`, `TestRFC5228HeaderTrimPreservesNonWSP`, `TestRFC5228HeaderTrimBoundaryOctets`, `TestASCIINumericHeaderWhitespace` |
| SIEVE-TST-09 | 5.8 | Normative | `not` inverts its test. | `TestSieve_Not`, `TestSieve_NotAnyofNested`, `TestRFC5228_AllofAnyofNested`, `TestDifferentialEvaluate` |
| SIEVE-TST-10 | 5.9 | Normative | `size :over n` is true when the message is larger than n octets and `:under n` when it is smaller; the size is that of the whole message. | `TestSieve_Size`, `TestSieve_Size_Quantifier`, `TestSieve_Size_IncludesAttachments`, `TestSieve_Size_IncludesHeaders`, `TestSieve_Size_UsesRawSizeWhenSet`, `TestEvaluate_SizeTest_BodyDepthBounded`, `TestRFC5228_SizeTest`, `TestSize_ReconstructionCountsRawHeaders`, `TestSize_ReconstructionPartBudget`, `TestDifferentialEvaluate` |
| SIEVE-TST-11 | 5.9 | Normative | `size` takes exactly one of `:over` and `:under` and a number. | `TestRFC5228_SyntaxErrors`, `TestDifferentialParse` |

## Match types, comparators and address parts (§2.6, §2.7)

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-MT-01 | 2.7.1 | Normative | `:is` requires equal strings, and is the default match type. | `TestRFC5228_MatchTypeIs`, `TestDifferentialEvaluate` |
| SIEVE-MT-02 | 2.7.1 | Normative | `:contains` is true when the key is a substring of the value; the empty key is a substring of every value. | `TestRFC5228_MatchTypeContains`, `TestDifferentialEvaluate` |
| SIEVE-MT-03 | 2.7.1 | Normative | `:matches` takes `*` for any run of characters (including none), `?` for exactly one octet, and `\` to take the next character literally; the whole value must match. | `TestSieve_MatchesStar`, `TestSieve_MatchesQuestionMark`, `TestSieve_MatchesAnchored`, `TestSieve_MatchesEscapedWildcard`, `TestSieve_MatchesQuestionMarkIsOctetNotRune`, `TestRFC5228_MatchesWildcards`, `TestMatchOne_ComparatorsAndRegex`, `TestDifferentialEvaluate` |
| SIEVE-MT-04 | 2.7.1 | Normative | A test takes at most one match type. | `TestValidateSieve_RejectsDuplicateOrInapplicableTags`, `TestRFC5228_SyntaxErrors`, `TestDifferentialParse` |
| SIEVE-MT-05 | 2.7.3 | Normative | `i;ascii-casemap` (the default) compares ASCII letters without regard to case and leaves other octets alone; `i;octet` compares octet for octet. | `TestSieve_ComparatorCasemapDefault`, `TestSieve_ComparatorOctetCaseSensitive`, `TestSieve_ComparatorAsciiCasemapAsciiOnly`, `TestComparator_ValidCombinationsStillParse`, `TestMatchOne_ComparatorsAndRegex`, `TestDifferentialEvaluate` |
| SIEVE-MT-06 | 2.7.3 | MUST | A comparator the implementation does not support is an error, whether or not the script declares it. | `TestComparator_UnknownRejected`, `TestRFC5228_SyntaxErrors` |
| SIEVE-MT-07 | 2.7.3, RFC 4790 §9.1.1 | Normative | `i;ascii-numeric` (declared with `require "comparator-i;ascii-numeric"`) compares leading digits as numbers of any length and supports only `:is`; `:contains` and `:matches` are errors. | `TestSieve_ComparatorAsciiNumeric`, `TestComparator_AsciiNumericRejectsSubstringMatchTypes`, `TestComparator_AsciiNumericIsSemantics`, `TestComparator_ValidCombinationsStillParse`, `TestDifferentialEvaluate` |
| SIEVE-MT-08 | 2.7.4 | Normative | `:localpart`, `:domain` and `:all` pick the part of an address that is compared; a test takes at most one. | `TestSieve_AddressLocalpart`, `TestSieve_AddressDomain`, `TestSieve_AddressAllDefault`, `TestSieve_EnvelopeDomainPart`, `TestRFC5228_SyntaxErrors`, `TestDifferentialEvaluate`, `TestDifferentialParse` |
| SIEVE-MT-09 | 2.7.3, RFC 4790 §9.1.1 | Normative | Numeric equality reads ASCII digits starting at the first octet without stripping whitespace; a non-digit first octet denotes positive infinity, and the first non-digit terminates a finite number. | `TestASCIINumericFirstOctet`, `TestASCIINumericRFCExamplesAndPrefixes`, `TestASCIINumericHeaderWhitespace`, `TestDifferentialNumericPrefixes` |
| SIEVE-MT-10 | RFC 4790 §3.1; RFC 5228 §2.7.3 | Normative | Comparator identifiers are US-ASCII; Unicode look-alikes must not select an ASCII comparator, including after encoded-character decoding. | `TestComparatorIdentifierASCII`, `TestComparatorIdentifierOctets`, `FuzzComparatorIdentifier` |

## Actions (§4) and their effect (§2.10)

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-ACT-01 | 4.3 | Normative | `keep` files the message into the default mailbox and cancels the implicit keep. | `TestSieve_DuplicateKeepDeliversOnce`, `TestRFC5228_SyntaxErrors`, `TestDifferentialEvaluate` |
| SIEVE-ACT-02 | 4.1 | Normative | With `require "fileinto"`, `fileinto` files the message into the named mailbox and cancels the implicit keep; filing into one mailbox twice delivers once. | `TestSieve_DuplicateFileintoDeliversOnce`, `TestSieve_DistinctFileintoDeliversEach`, `TestSieve_FileintoCopyDistinctFromConsuming`, `TestSieve_FileintoThenDiscardStillDelivers`, `TestSieve_DiscardThenFileintoStillDelivers`, `TestDifferentialEvaluate` |
| SIEVE-ACT-03 | 4.2 | Normative | `redirect` sends the message to one address and cancels the implicit keep; an address with control characters is an error; redirecting to one address twice delivers once. | `TestSieve_Redirect`, `TestSieve_DuplicateRedirectDeliversOnce`, `TestSieve_DistinctRedirectDeliversEach`, `TestValidateSieve_RejectsRedirectControlChars`, `TestValidateSieve_AcceptsNormalRedirect`, `TestRFC5228RedirectAddressSyntax`, `TestRFC5228RedirectAddressPreserved`, `TestRFC5228RedirectDecodedAddress`, `TestRFC5228_SyntaxErrors`, `TestDifferentialEvaluate` |
| SIEVE-ACT-04 | 4.2, 10 | MUST | Detect and break redirect loops; a detected loop is an error. | n/a: the host Executor owns outgoing delivery and loop detection; it must signal a detected loop as a runtime error, not silently send again. |
| SIEVE-ACT-05 | 4.4 | Normative | `discard` cancels the implicit keep, drops the message if nothing else delivers it, and does not end the script or withdraw other actions. | `TestSieve_Discard`, `TestSieve_BareDiscardDropsMessage`, `TestSieve_DiscardDoesNotStopScript`, `TestSieve_DiscardThenFileintoStillDelivers`, `TestSieve_FileintoThenDiscardStillDelivers`, `TestSieve_DiscardThenStopDrops`, `TestDifferentialEvaluate` |
| SIEVE-ACT-06 | 2.10.2 | MUST | Unless an action cancels it, the implicit keep delivers the message to the default mailbox, also for an empty script and a script that selects no action. | `TestSieve_NoActionImpliesImplicitKeep`, `TestSieve_EmptyScriptImpliesImplicitKeep`, `TestSieve_StopBeforeActionKeepsImplicitly`, `TestSieve_CopyImplicitKeep`, `TestSieve_RedirectCopyTag`, `TestDifferentialEvaluate`, `TestDifferentialStop` |
| SIEVE-ACT-07 | 2.10.6 | MUST | When an error occurs while the script runs, the message is kept rather than lost. | `TestSieve_RuntimeErrorKeepsMessage`, `TestEvaluate_NilMessage_DoesNotPanic`, `TestEvaluate_NilMessage_NilScriptStillGuarded`, `TestEvaluate_PanicWithErrorValue` |
| SIEVE-ACT-08 | 2.10.4 | Normative | Actions run in script order, each tests the message as received (actions do not change it), and a test that cannot match does not stop the script. | `TestSieve_CombinedExtensions`, `TestSieve_InvalidRegex_Skipped` |
| SIEVE-ACT-09 | 6.1 | Normative | `fileinto`, `reject`, `envelope`, `vacation` and the other extensions are only usable once declared, each under its capability string. | `TestRequire_AcceptsProperlyDeclaredScripts`, `TestRequire_RejectsUndeclaredExtensionUse` |
| SIEVE-ACT-10 | 4.3, 8.2 | Normative | `keep` takes no positional arguments of its own; `keep "x";` is an error. The only argument accepted is `:flags` (RFC 5232 §5), once `imap4flags` is declared. | `TestKeep_RefusesArguments` |

## Beyond RFC 5228: the extensions the package implements

The capability inventory is checked by `TestExtensionCapabilityInventory`.
`fileinto` and `envelope` belong to RFC 5228 (SIEVE-ACT-02 and SIEVE-TST-05),
`encoded-character` to SIEVE-LEX-06, and the three comparators to SIEVE-MT-05/07.
`reject`, `ereject`, `vacation`, `vacation-seconds`, `copy`, `imap4flags`, `body`
and `mailbox` have separate sections below. `regex` and `notify` are local
extensions; their names do not establish conformance to an RFC or draft.

The broad overview rows below retain their existing test mappings. In
particular, SIEVE-EXT-02 proves flag callbacks, not delivery of flags: FLAGS-18/19
record that gap. FLAGS-17, BODY-07 and MAILBOX-08 still need owner decisions.
Relational, subaddress and variables are not implemented; numeric equality,
ordinary address parts and literal strings do not provide those capabilities.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-EXT-01 | RFC 5230, 6131 | Normative | `vacation` (with `:days`, `:seconds`, `:subject`, a reason) asks the host to send an auto-reply to the envelope sender; it is suppressed for the null reverse-path. | `TestSieve_Vacation_Basic`, `TestSieve_Vacation_DefaultDays`, `TestSieve_Vacation_Conditional`, `TestSieve_Vacation_UsesEnvelopeSender`, `TestSieve_Vacation_MultiLine`, `TestSieve_Vacation_SecondsGranularity`, `TestSieve_Vacation_NullReversePathSuppressed`, `TestVacation_ReplyTo`, `TestExtensionArguments_Parse` |
| SIEVE-EXT-02 | RFC 5232 | Normative | `setflag`, `addflag` and `removeflag` report flag changes through the host callback; the unimplemented delivery semantics are recorded in FLAGS-18/19. | `TestSieve_SetAndAddFlag`, `TestSieve_RemoveFlag`, `TestSieve_SetFlagEscaped`, `TestExtensionArguments_Parse` |
| SIEVE-EXT-03 | RFC 5173 | Normative | `body` tests the text of the message body (HTML tags stripped, the plain part of a multipart preferred) with a match type and comparator. | `TestSieve_BodyContains`, `TestSieve_BodyContains_NoMatch`, `TestSieve_BodyContains_HTMLStripped`, `TestSieve_BodyContains_MultipartPlainPreferred`, `TestSieve_BodyIs`, `TestExtractBodyText_PlainText`, `TestExtractBodyText_HTML`, `TestExtractBodyText_Multipart`, `TestExtractBodyText_MultipartHTMLOnly`, `TestStripHTMLTags`, `TestEvaluate_BodyTest_DepthBounded`, `TestEvaluate_BodyTest_ContentDepthBounded`, `TestEvaluate_BodyTest_DepthWithinCapStillMatches`, `TestEvaluate_BodyTest_PartCountBounded` |
| SIEVE-EXT-04 | RFC 3894 | Normative | `:copy` on `fileinto` and `redirect` leaves the implicit keep in effect. | `TestSieve_RedirectCopyTag`, `TestSieve_CopyImplicitKeep`, `TestSieve_FileintoCopyDistinctFromConsuming`, `TestExtensionArguments_Parse`, `TestDifferentialEvaluate` |
| SIEVE-EXT-05 | RFC 5429, 5490 | Normative | `reject` refuses the message with a reason (each clause of RFC 5429 is in the next section); `fileinto :create` asks the host to create the mailbox. | `TestSieve_Reject`, `TestSieve_FileintoCreate`, `TestExtensionArguments_Parse` |
| SIEVE-EXT-06 | non-standard | Normative | `:regex` is a match type of the package, not of any RFC; an invalid expression never matches. | `TestSieve_HeaderRegex`, `TestSieve_HeaderRegex_NoMatch`, `TestSieve_HeaderRegex_CaseInsensitive`, `TestSieve_BodyRegex`, `TestSieve_AddressRegex`, `TestSieve_EnvelopeRegex`, `TestMatchOne_ComparatorsAndRegex` |
| SIEVE-EXT-07 | non-standard | Normative | `notify` asks the host to send a notification. | `TestSieve_Notify_Basic`, `TestSieve_Notify_Conditional`, `TestSieve_Notify_NoMatch`, `TestSieve_Notify_MultiLine`, `TestExtensionArguments_Parse` |


## RFC 5429: `reject` and `ereject`

Each clause of [RFC 5429](https://www.rfc-editor.org/rfc/rfc5429). The library
decides whether a script refuses the message and with what reason, and reports
it as `Outcome{Disposition: Reject, RejectReason: …}`; how the refusal reaches
the sender (an SMTP/LMTP 5XX reply, a DSN or an MDN) is the host's job, so those
clauses are n/a here. A section number alone is RFC 5429.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| EREJECT-01 | 2.1 | Normative | `ereject <reason: string>`, under the capability `ereject`: one reason string is required. | `TestReject_RequiresReason` |
| EREJECT-02 | 2.1 | Normative | `ereject` cancels the implicit keep and refuses the message; the reason is a UTF-8 string. | `TestReject_RefusesAndCancelsImplicitKeep` |
| EREJECT-03 | 2.1 | MUST | Carry out the most preferable refusal possible (a 5XX reply over SMTP/LMTP, else a non-delivery report) and fall back to the lesser one if it fails. | n/a: the host's job; the library hands it the refusal and the reason. |
| EREJECT-04 | 2.1 | SHOULD | Log each case where no non-delivery report is sent, and why. | n/a: the host's job, it decides whether a report is sent. |
| EREJECT-05 | 2.1 | MUST NOT | `ereject` is not available where protocol-level refusal is impossible (an MUA), and is available everywhere `reject` is. | n/a: the library runs in a server (MDA/MTA) that can refuse at protocol level, and offers `ereject` together with `reject`. |
| EREJECT-06 | 2.1.1 | MUST | Refusing at SMTP/LMTP level uses a 5XX reply (550 SHOULD be used); a reason that cannot be sent as is is replaced; after the transaction a DSN is sent, following RFC 3464 and RFC 3462, with action "failed" and the reason. | n/a: the host's job. |
| REJECT-01 | 2.2 | Normative | `reject <reason: string>`, under the capability `reject`: one reason string is required, and a bare `reject;` is an error. | `TestReject_RequiresReason` |
| REJECT-02 | 2.2 | Normative | `reject` cancels the implicit keep and refuses the message; the reason is a UTF-8 string, kept exactly. | `TestReject_RefusesAndCancelsImplicitKeep`, `TestSieve_Reject` |
| REJECT-03 | 2.4 | MUST | Never execute more than one `reject` (or `ereject`) in a script. | `TestReject_OnlyOneRuns` |
| REJECT-04 | 2.2.1 | MUST | `reject` refuses by a failure MDN (disposition "deleted", sent automatically, with the reason), or MAY refuse at SMTP/LMTP level under the conditions of §2.2; MDN logging SHOULD be done. | n/a: the host's job. |
| REJECT-05 | 2.3 | MUST NOT | `reject` is not silently upgraded to `ereject`. | `TestReject_OutcomeDistinguishesExtendedRefusal` |
| REJECT-06 | 2.3 | SHOULD | Script generators use `ereject` for anti-spam and anti-virus refusals, and check the target supports it before upgrading. | n/a: addressed to script generators, not to an interpreter. |
| REJECT-07 | 2.4 | MUST | `reject` is incompatible with `vacation`. | `TestReject_VacationIncompatible`, `TestReject_VacationExclusiveBranches` |
| REJECT-08 | 2.4 | Normative | A `reject` cancels the implicit keep. | `TestReject_RefusesAndCancelsImplicitKeep` |
| REJECT-09 | 2.4 | SHOULD NOT | Do not permit `reject` together with actions that deliver the message (`keep`, `fileinto`, `redirect`). | n/a: permitted, as §2.4 allows for archiving; a delivering action before the `reject` has already been handed to the host, and the `Outcome` is still Reject. |
| REJECT-10 | 2.4 | MUST NOT | An action that modifies the body has no effect on the content of a DSN or MDN. | n/a: the library implements no body-modifying action, and the DSN or MDN is the host's. |
| REJECT-11 | 2.5 | MUST | A multi-line reason is returned as a multi-line SMTP/LMTP reply, no line over the SMTP limit (CRLFs MAY be inserted), with enhanced status code 5.7.1 or 5.7.0 if RFC 2034 is supported. | n/a: the host's job; the library returns the reason, CRLFs included, unchanged. |


## RFC 5230 vacation extension

[RFC 5230](https://www.rfc-editor.org/rfc/rfc5230) requirements, including
normative argument grammar and message semantics. Section numbers in this table
refer to RFC 5230. The existing SIEVE-EXT-01 row remains an overview (its
`:seconds` case belongs to RFC 6131). The §4.7 reject/refuse incompatibility is
tracked exclusively by REJECT-07 above and PR #56; this section does not change
that behavior or duplicate its row.

The library selects actions; the host owns delivery and persistent tracking.
The host must supply the evaluated user's final envelope recipients and original
headers, apply the
tracking rules below, validate and serialize the response, and add its required
headers. Host responsibilities are not claims of implementation conformance.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| VACATION-01 | 3 | Normative | The capability identifier is `vacation`; the action requires its declaration. | `TestVacation_Syntax` |
| VACATION-02 | 4 | Normative | A reason string is required, including when empty; no extra positional argument is allowed. | `TestVacation_Syntax` |
| VACATION-03 | 4, 4.1 | Normative | `:days` takes a Sieve number, in days. | `TestVacation_Syntax`, `TestVacation_DaysBounds` |
| VACATION-04 | 4, 4.3 | Normative | `:subject` takes a string; preserve its value and distinguish an explicit empty subject from omission for the host. | `TestVacation_Syntax`, `TestVacation_CallbackArguments` |
| VACATION-05 | 4.3 | SHOULD | Check the syntax of the `:from` mailbox-list and report an invalid value as an error. | `TestVacation_Syntax` |
| VACATION-06 | 4, 4.5 | Normative | `:addresses` takes a string-list of additional recipient identities. | `TestVacation_Syntax`, `TestVacation_PersonalRecipient` |
| VACATION-07 | 4, 4.4 | Normative | `:mime` takes no argument and marks the reason as a MIME entity including headers and content. | `TestVacation_Syntax`, `TestVacation_CallbackArguments` |
| VACATION-08 | 4, 4.2 | Normative | `:handle` takes a string; retain the explicit handle, including an empty one, for the host's tracking. | `TestVacation_Syntax`, `TestVacation_CallbackArguments` |
| VACATION-09 | 4; RFC 5228 §2.4.2 | Normative | Only defined tags are allowed and each may occur at most once. | `TestVacation_Syntax` |
| VACATION-10 | 4.1 | Normative | The normal minimum is one day; values below it use that minimum. | `TestVacation_DaysBounds` |
| VACATION-11 | 4.1 | MAY | A site may choose a different minimum, strictly greater than zero. | n/a: not selected: the library uses the normal one-day minimum; it exposes no site minimum setting. |
| VACATION-12 | 4.1 | MAY | A site may define a maximum; this implementation selects 365 days. | `TestVacation_DaysBounds` |
| VACATION-13 | 4.1 | MUST | A configured maximum is greater than seven days (365 here). | `TestVacation_DaysBounds` |
| VACATION-14 | 4.1 | SHOULD | A configured maximum is greater than thirty days (365 here). | `TestVacation_DaysBounds` |
| VACATION-15 | 4.1 | Normative | Omitting `:days` uses the greater of seven days and the minimum; a value above the maximum is clamped before duration conversion. | `TestVacation_DaysBounds` |
| VACATION-16 | 4, 4.2 | Normative | The response destination is the SMTP envelope sender; never substitute From or Reply-To. Suppress null or unavailable reverse-paths. | `TestVacation_EnvelopeDestination`, `TestSieve_Vacation_NullReversePathSuppressed` |
| VACATION-17 | 4.2, 8 | MUST NOT | Send the same response to a sender more frequently than the interval, unless the script changes. | n/a: host's job: Executor owns persistent tracking storage and time; Vacation supplies Interval, ReplyTo and response identity arguments. |
| VACATION-18 | 4.2 | Normative | For tracking, identical explicit handles identify the same response even when other arguments differ. | n/a: host's job: the library reports Handle and HandleSet without discarding an empty handle (proved by VACATION-08); Executor performs tracking. |
| VACATION-19 | 4.2 | Normative | Without a handle, synthesize response identity from the original :subject, :from, :mime and reason arguments. | n/a: host's job: the unchanged arguments, including SubjectSet, are reported (VACATION-04, VACATION-07, VACATION-26); Executor constructs its storage key. |
| VACATION-20 | 4.2 | REQUIRED | Different response identities can elicit separate replies even within the interval for the same sender. | n/a: host's job: persistent de-duplication is outside this evaluator; it reports each selected action. |
| VACATION-21 | 4.2 | MUST NOT | Expand variables before deriving the response-tracking identity. | n/a: the variables extension (RFC 5229) is unsupported; no variable expansion occurs. |
| VACATION-22 | 4.2 | MAY | Reset previous-response records when a script changes. | n/a: host's job: script lifecycle and persistent tracking records are host-owned. |
| VACATION-23 | 4.2 | MUST NOT | Limit remembered responses to fewer than 1000. | n/a: host's job: the library has no persistent response store or storage limit. |
| VACATION-24 | 4.2 | SHOULD | When response tracking is bounded, discard the oldest records first. | n/a: host's job: eviction policy belongs to the persistent response store. |
| VACATION-25 | 4.3 | MUST | Encode subject text as RFC 2047 encoded words if and only if it contains non-ASCII characters. | n/a: host's job: the library passes UTF-8 Subject unchanged and generates no response headers. |
| VACATION-26 | 4.3, 5.7 | Normative | Preserve the supplied :from mailbox-list and take the response body from the reason string. | `TestVacation_CallbackArguments` |
| VACATION-27 | 4.3, 5.3 | MUST | When :subject is omitted, generate a default, normally "Auto: " followed by the original subject. | n/a: host's job: SubjectSet identifies omission; Executor generates the outgoing subject using the original Message. |
| VACATION-28 | 4.3 | MAY | Restrict permitted :from identities, preferably ignoring disallowed identities rather than failing. | n/a: not selected: the library checks syntax but leaves account/identity authorization to the host. |
| VACATION-29 | 4.4, 5 | Normative | Without :mime the reason is UTF-8 text; with :mime it is a complete MIME entity. | `TestVacation_CallbackArguments` |
| VACATION-30 | 4.5, 8 | MUST NOT | Respond unless a recipient identity occurs in To, Cc, Bcc, Resent-To, Resent-Cc or Resent-Bcc; the final envelope recipient is an identity. | `TestVacation_PersonalRecipient` |
| VACATION-31 | 4.5 | Normative | `:addresses` adds recipient identities to those already known, rather than replacing them. | `TestVacation_PersonalRecipient` |
| VACATION-32 | 4.6 | MAY | Suppress replies based on other headers or content indicating a response is inappropriate; bulk/list/junk Precedence is suppressed. | `TestVacation_Suppression` |
| VACATION-33 | 4.6 | MUST | Maintain an implementation-defined list of system addresses excluded from replies (MAILER-DAEMON, LISTSERV, majordomo, owner-* and *-request here). | `TestVacation_Suppression` |
| VACATION-34 | 4.6 | MUST NOT | Send vacation replies to an address on the system-address exclusion list. | `TestVacation_Suppression` |
| VACATION-35 | 4.6 | SHOULD NOT | Respond to messages carrying any List-Id, List-Help, List-Subscribe, List-Unsubscribe, List-Post, List-Owner or List-Archive field. | `TestVacation_Suppression` |
| VACATION-36 | 4.6 | SHOULD NOT | Respond when Auto-Submitted has a value other than no, including auto-generated and auto-replied. | `TestVacation_Suppression` |
| VACATION-37 | 4.7 | Normative | Vacation does not cancel the implicit keep. | `TestVacation_ActionInteraction` |
| VACATION-38 | 4.7 | MUST | A second executed vacation action fails the script with an error; an unexecuted branch does not count. | `TestVacation_ActionInteraction` |
| VACATION-39 | 4.7 | MUST NOT | Treat vacation with discard, keep, fileinto or redirect as an error. | `TestVacation_ActionInteraction` |
| VACATION-40 | 5 | MUST | Accept UTF-8 input for response content. | `TestVacation_CallbackArguments` |
| VACATION-41 | 5 | MAY | Transcode generated responses to other character sets. | n/a: host's job: response serialization and character-set selection are outside the library. |
| VACATION-42 | 5 | SHOULD | Reject :mime actions whose MIME headers contain 8-bit material; outgoing MIME headers conform to MIME conventions. | n/a: host's job: Executor validates the supplied MIME entity before sending; the library reports Body and MIME without serializing a message. |
| VACATION-43 | 5.1 | SHOULD | Use a null SMTP MAIL FROM for the response. | n/a: host's job: the library opens no SMTP transaction. |
| VACATION-44 | 5.1 | SHOULD | Use NOTIFY=NEVER on RCPT TO when the SMTP NOTARY extension is available. | n/a: host's job: SMTP capability negotiation and transport are outside the library. |
| VACATION-45 | 5.2 | SHOULD | Set Date to the response generation time. | n/a: host's job: the library generates no response headers. |
| VACATION-46 | 5.3 | SHOULD | Use an appropriate fixed subject when both :subject and the original Subject are absent. | n/a: host's job: SubjectSet identifies omission; the host has the original Message and creates the reply headers. |
| VACATION-47 | 5.4 | SHOULD | Default the response From to the script owner unless :from overrides it. | n/a: host's job: the library reports From; the host knows the script owner and generates headers. |
| VACATION-48 | 5.5 | SHOULD | Set response To to the response recipient. | n/a: host's job: ReplyTo supplies the destination; Executor constructs the outgoing To header. |
| VACATION-49 | 5.6; RFC 3834 | SHOULD | Include Auto-Submitted: auto-replied in the outgoing vacation response. | n/a: host's job: the library reports an action and never constructs an outgoing message or headers. |
| VACATION-50 | 5.8 | MUST | Set In-Reply-To to the original Message-ID when it exists. | n/a: host's job: the host retains the original Message and generates the response headers. |
| VACATION-51 | 5.8 | SHOULD | Update References with the original Message-ID when it exists. | n/a: host's job: the host generates response threading headers from the original Message. |
| VACATION-52 | 5.8 | Normative | Without an original Message-ID, In-Reply-To and References need not be generated or changed. | n/a: host's job: optional threading headers are generated by Executor, not the evaluator. |


## RFC 3894 copy extension

[RFC 3894](https://www.rfc-editor.org/rfc/rfc3894) requirements, clause by
clause. Section numbers in this table refer to RFC 3894. The SIEVE-EXT-04 row
above remains the overview, and SIEVE-ACT-06 covers the implicit keep from the
side of RFC 5228.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| COPY-01 | 2; RFC 5228 §3.2 | Normative | The capability string is `copy`; `:copy` is an error unless the script declares it. | `TestCopy_RequiresCapability`, `TestRequire_RejectsUndeclaredExtensionUse` |
| COPY-02 | 3; RFC 5228 §2.4.2 | Normative | `:copy` is an optional tag of `fileinto` and `redirect` before their string argument, given at most once, and of no other command or test. | `TestCopy_Syntax`, `TestKeep_RefusesArguments`, `TestValidateSieve_RejectsDuplicateOrInapplicableTags` |
| COPY-03 | 3 | Normative | A `fileinto` or `redirect` tagged `:copy` does not cancel the implicit keep, and does not restore one another action cancelled. | `TestSieve_CopyImplicitKeep` |
| COPY-04 | 3 | Normative | The tagged command files or redirects a copy in addition to whatever else happens to the message. | `TestCopy_InAdditionToOtherActions`, `TestSieve_CopyImplicitKeep`, `TestSieve_FileintoCopyDistinctFromConsuming` |
| COPY-05 | 3 | Normative | The example script of §3 is accepted and files a copy while keeping the implicit keep. | `TestCopy_RFCExample` |
| COPY-06 | 4 | MUST | Write access to Sieve scripts is protected as strongly as read/write access to mail. | n/a: host's job: the library parses a script it is given and stores none. |


## RFC 5232 imap4flags extension

[RFC 5232](https://www.rfc-editor.org/rfc/rfc5232) requirements, clause by
clause. Section numbers in this table refer to RFC 5232. The SIEVE-EXT-02 row
above remains the overview. The library passes each `setflag`, `addflag` and
`removeflag` to `Executor.Flag` with its list of flags cleaned up as §2
requires; the host holds the flag set (the "internal variable") and applies it
to the message it stores. The `variables` extension is not supported, so the
rows about variable names cover only the error they must cause.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| FLAGS-01 | 1; RFC 5228 §3.2 | Normative | The capability string is `imap4flags`; its actions and the `:flags` tagged argument are errors unless the script declares it. | `TestFlags_RequiresCapability` |
| FLAGS-02 | 1, 3.1–3.3 | MUST | `setflag`, `addflag` and `removeflag` are implemented: each reaches the host as its own operation, in script order. | `TestFlags_ActionsReachHost`, `TestSieve_SetAndAddFlag`, `TestSieve_RemoveFlag` |
| FLAGS-03 | 1, 3 | MUST NOT | Without the `variables` extension, a variable name before the list of flags is an error. | `TestFlags_VariableNameRefused` |
| FLAGS-04 | 5 | Normative | `:flags` takes one list of flags and is a tag of `keep` and `fileinto` only. | `TestFlags_FlagsTagSyntax` |
| FLAGS-05 | 2, 3.3 | MUST | Empty strings in a list of flags are ignored. | `TestFlags_ListNormalised` |
| FLAGS-06 | 2 | Normative | A string holding a space-separated list of flag names is the same as a string list of those flags. | `TestFlags_ListNormalised` |
| FLAGS-07 | 2 | MUST | Spaces are always delimiters; a run of spaces counts as one, and leading and trailing spaces are ignored. | `TestFlags_ListNormalised` |
| FLAGS-08 | 2 | SHOULD | Each flag is checked against the IMAP grammar (RFC 3501 §9 `flag`), so a non-ASCII flag name is invalid. | `TestFlags_InvalidIgnored` |
| FLAGS-09 | 2 | MUST | A flag that fails the check is ignored. | `TestFlags_InvalidIgnored` |
| FLAGS-10 | 2 | MUST | `\Recent` and any other system flag a client cannot set in IMAP are ignored. | `TestFlags_SystemFlags`, `TestSieve_SetFlagEscaped` |
| FLAGS-11 | 3 | MAY | An action may reorder flags, change their case or drop duplicates without changing the set; the library drops a repeated flag (compared case-insensitively) and writes system flags in their usual case. | `TestFlags_DuplicatesRemoved`, `TestFlags_SystemFlags` |
| FLAGS-12 | 3.1, 3.2 | Normative | The examples of §3.1 and §3.2 that need no variables give the results the RFC describes. | `TestFlags_RFCExamples` |
| FLAGS-13 | 3.3 | MUST | Without automatic duplicate removal, `removeflag` removes every occurrence of a flag; removing a flag that is not set is not an error. | `TestFlags_DuplicatesRemoved`, `TestSieve_RemoveFlag` |
| FLAGS-14 | 6 | Normative | The extension does not change the implicit keep. | `TestFlags_ImplicitKeepUnchanged` |
| FLAGS-15 | 3 | MUST NOT | Scripts do not assume the order or the case of flags in a list. | n/a: a requirement on scripts, not on the implementation. |
| FLAGS-16 | 3 | Normative | Without a variable name the actions work on the internal variable, empty when the script starts; `addflag` adds, `removeflag` removes and `setflag` replaces. | n/a: host's job: the host holds the set and applies each operation `Executor.Flag` reports. |
| FLAGS-17 | 1, 4 | MUST | The `hasflag` test (match type defaulting to `:is`, comparator defaulting to `i;ascii-casemap`, `:count` counting distinct flags). | n/a: not supported yet, see rest-mail/rest-mail-server#374: the library does not hold the flag set, so it cannot test it. |
| FLAGS-18 | 3, 5 | MUST | `keep` and `fileinto` (and the implicit keep) deliver with the internal variable's flags, or with only the `:flags` list when it is given. | n/a: not supported yet, see rest-mail/rest-mail-server#374: `:flags` is parsed and then dropped, as `Executor.Keep` and `Executor.FileInto` carry no flags. |
| FLAGS-19 | 3; RFC 5228 §2.10.3 | MUST | When repeated `keep` or `fileinto` deliveries are merged, the last list of flags wins. | n/a: not supported yet, follows FLAGS-18. |
| FLAGS-20 | 5 | MUST | Flags the mailbox cannot store are ignored, and failing to store one is not a runtime error. | n/a: host's job: the library stores nothing, and `Executor.Flag` returns no error. |
| FLAGS-21 | 7 | Normative | The extension sets flags only on the message being processed, never on other messages in the store. | n/a: host's job: `Executor.Flag` is called for the current message only. |


## RFC 5173 body extension

[RFC 5173](https://www.rfc-editor.org/rfc/rfc5173) requirements, clause by
clause. Section numbers in this table refer to RFC 5173. The SIEVE-EXT-03 row
above remains the overview. The host supplies the body as a tree of `Body`
parts, each decoded to UTF-8 text; the library picks the parts a body transform
names and matches them. `:text` (the default) matches one string: the first
`text/plain` part, or else the first `text/html` part with its tags stripped.
`:content` matches each part of a listed type as its own string.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| BODY-01 | 3; RFC 5228 §3.2 | Normative | The capability string is `body`; the test is an error unless the script declares it. | `TestBody_RequiresCapability` |
| BODY-02 | 4; RFC 5228 §2.6.2, §2.6.3 | Normative | The comparator, match type and body transform come in any order; the defaults are `:is` and `i;ascii-casemap`. | `TestBody_ArgumentsAnyOrderAndDefaults` |
| BODY-03 | 5 | Normative | A body test takes at most one of `:raw`, `:text` and `:content <string-list>`, and no other test takes them. | `TestBody_TransformSyntax` |
| BODY-04 | 5 | Normative | The default body transform is `:text`. | `TestBody_DefaultTransformIsText` |
| BODY-05 | 4 | Normative | A message that is a header with no empty line after it has no body: every body test is false, even one for the empty string. | `TestBody_NoBodyIsFalse` |
| BODY-06 | 4 | Normative | The body is matched as a whole, not line by line, so the strings matched hold CRLFs. | `TestBody_NotLineByLine` |
| BODY-07 | 5.1 | MUST | `:raw` matches the whole undecoded body as one string: transfer encodings stay, and boundaries and the MIME headers of nested parts are part of the content. | n/a: not supported yet, see rest-mail/rest-mail-server#374: `Message` holds no undecoded body, so `:raw` matches the same text as `:text`. |
| BODY-08 | 5.1 | MUST NOT | A message with syntax errors is still filtered with `:raw`. | n/a: host's job: the host parses the message into `Message`; evaluation never refuses one. |
| BODY-09 | 5.2 | Normative | A `:content` type of `""` matches every type, a bare type each of its subtypes, and `type/subtype` only itself; one that begins or ends with a slash or has two slashes matches none. | `TestBody_ContentTypeRules`, `FuzzBodyContentTypeSelection` |
| BODY-10 | 5.2 | Normative | The search for `:content` parts descends into multipart and `message/rfc822` parts; every part of a matching type is searched, and the test is true if any part matches any key. | `TestBody_ContentRecursive`, `TestBody_RFCExample` |
| BODY-11 | 5.2 | Normative | A matching multipart part is searched in its prologue and its epilogue, as two strings; a matching `message/rfc822` part in its header only. | n/a: host's job: `Body.Content` of a container is matched as one string, so the host puts the prologue and epilogue, or the nested header, there. |
| BODY-12 | 5.2 | MUST NOT | A key does not match across part boundaries, and the MIME headers of the containing part are not in the data. | `TestBody_NoMatchAcrossParts`, `TestBody_RFCExample` |
| BODY-13 | 5.2 | MUST | The quoted-printable, base64, 7bit, 8bit and binary transfer encodings are decoded, and US-ASCII, ISO-8859-1 and the US-ASCII subset of ISO-8859-* are converted to UTF-8. | n/a: host's job: `Body.Content` is decoded UTF-8 text, as its doc comment says. |
| BODY-14 | 5.2 | SHOULD NOT | A NUL octet does not end the content being compared. | `TestBody_NULDoesNotTerminate` |
| BODY-15 | 5.3 | MUST | All the text taken from one non-container part is in the same string. | `TestBody_TextKeepsPartTogether` |
| BODY-16 | 5.3 | MAY | `:text` may strip mark-up; the library strips HTML tags. | `TestBody_DefaultTransformIsText`, `TestSieve_BodyContains_HTMLStripped` |
| BODY-17 | 6 | MUST NOT | With the `variables` extension, wildcards in a body test do not set match variables. | n/a: the `variables` extension is not supported. |
| BODY-18 | 5.1–5.3 | Normative | The example message and scripts of §5 give the results the RFC describes. | `TestBody_RFCExample` |
| BODY-19 | 5.2; RFC 2045 §§2, 5.1 | Normative | MIME type selection is ASCII case-insensitive; Unicode case look-alikes do not select an ASCII type, independently of the body comparator. | `TestBody_ContentTypeASCIICase`, `FuzzBodyContentTypeSelection` |


## RFC 5490 — mailbox status and creation (§3.1–3.2)

[RFC 5490](https://www.rfc-editor.org/rfc/rfc5490) registers three independent
capabilities: `mailbox`, `mboxmetadata` and `servermetadata`. The library accepts
`require "mailbox"` for `fileinto :create`, but does not implement the other
half of that capability, `mailboxexists`. This is partial support, not full RFC
5490 conformance. Adding a mailstore query needs a host API decision; see
rest-mail/rest-mail-server#374. The RFC's §3.1 example depends on that missing
test and cannot yet be evaluated.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| MAILBOX-01 | 3.2 | Normative | `:create` requires `mailbox`; the `fileinto` action independently requires `fileinto`. | `TestMailbox_RequiresCapability` |
| MAILBOX-02 | 3.2; RFC 5228 §2.4.2 | Normative | `:create` is an optional, argument-free tag before the single mailbox string, at most once, on `fileinto` only. | `TestMailbox_CreateSyntax` |
| MAILBOX-03 | 3.2 | Normative | A tagged action requests creation of its named mailbox; an untagged action does not. | `TestMailbox_CreateCallback` |
| MAILBOX-04 | 3.2; RFC 3894 §3 | Normative | `:create` controls creation, without changing the action's implicit-keep or `:copy` semantics. | `TestMailbox_CreateCallback` |
| MAILBOX-05 | 3.2 | Normative | Create a missing mailbox before delivering into it. | n/a: host's job: `Executor.FileInto` receives the folder and creation request; the library has no storage access. |
| MAILBOX-06 | 3.2 | Normative | If the mailbox already exists, ignore the creation request. | n/a: host's job: only the host can determine whether the mailbox exists. |
| MAILBOX-07 | 3.2; RFC 5228 §2.10.6 | Normative | A creation failure is an execution error, with fail-safe keep rather than message loss. | `TestMailbox_CreationFailure` |
| MAILBOX-08 | 3.1 | Normative | `mailboxexists` accepts a string list and is true only when every mailbox exists and permits delivery by the script user. | n/a: not supported yet, see rest-mail/rest-mail-server#374: the evaluator has no mailstore query API; this also prevents running the RFC's example. |
| MAILBOX-09 | 3.1 | Normative | For IMAP storage, delivery eligibility means READ-WRITE without ACL, or the user's p or i ACL right. | n/a: follows MAILBOX-08; the host must check the evaluated user's rights, not just existence. |
| MAILBOX-10 | 3.1 | Normative | An existence/permission check does not guarantee a subsequent delivery will succeed, such as when over quota. | n/a: follows MAILBOX-08; delivery and quota enforcement are the host's responsibility. |
| MAILBOX-11 | 3.3–3.4 | Normative | `mboxmetadata` provides mailbox annotation matching and existence tests, using the script user's permissions and private annotations. | n/a: unsupported separate capability; the parser rejects `require "mboxmetadata"`. |
| MAILBOX-12 | 4–5 | Normative | `servermetadata` provides server annotation matching and existence tests; annotation access uses the script user's context, handles temporary failures and preserves script-level confidentiality. | n/a: unsupported separate capability; no annotations are retrieved and the parser rejects `require "servermetadata"`. |


## Capability boundary and unimplemented extensions

These rows distinguish implemented behavior from unsupported, independently
negotiated extensions. Rejection tests in EXT-CAP-01 prove the boundary, not the
missing features. The unsupported rows are not clause-by-clause audits of RFCs
this library does not implement.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| EXT-CAP-01 | RFC 5228 §3.2 | MUST | Accept exactly the inventoried capabilities; reject unsupported capability declarations and syntax rather than execute a partial script. | `TestExtensionCapabilityInventory` |
| RELATIONAL-01 | RFC 5231 §1, §4.1–4.2 | Normative | `relational` adds ordered value comparisons and entity counts. | n/a: not implemented; `require "relational"`, `:value` and `:count` are rejected; `i;ascii-numeric` supports equality only. |
| SUBADDRESS-01 | RFC 5233 §3–4 | Normative | `subaddress` adds user/detail address parts using the encompassing mail system's encoding. | n/a: not implemented; declaration and `:user`/`:detail` are rejected; there is no host subaddress policy API. |
| VARIABLES-01 | RFC 5229 §2–5 | Normative | `variables` enables interpolation, assignment and string-value tests. | n/a: not implemented; declaration, `set` and `string` are rejected; `${name}` stays literal without the extension. |
| ENOTIFY-01 | RFC 5435 §2 | Normative | The standardized notification capability is `enotify`. | n/a: not implemented; `enotify` is rejected; the local `notify` action only forwards method/message and does not implement RFC 5435 validation, options, tests or transport policy. |


## Local regex contract (no RFC conformance claim)

`regex` selects Go's `regexp` syntax, including its lack of backreferences.
It is not a claim to implement the Sieve regex Internet-Draft. These rows pin
observable package behavior separately from normative RFC requirements. The default regex mode uses Unicode case folding from Go,
unlike the ASCII-only folding used by the core match types.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| REGEX-01 | Local extension | Normative | `:regex` requires `regex`; matching searches within a value unless the expression supplies anchors. | `TestRegex_ExtensionContract` |
| REGEX-02 | Local extension | Normative | `i;octet` preserves case; the default comparator enables Go regular-expression case-insensitive matching. | `TestRegex_ExtensionContract` |
| REGEX-03 | Local extension | Normative | Invalid or unsupported expressions do not match and do not stop later actions. | `TestRegex_ExtensionContract` |


## RFC 6131 — vacation seconds

[RFC 6131](https://www.rfc-editor.org/rfc/rfc6131) extends the existing vacation
callback. The site maximum is 2147483647 seconds, applied before conversion to
`time.Duration`. The RFC's range prose inconsistently includes/excludes 2^31;
the parser accepts that value too and caps it. No persistent timer is maintained
by this library.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SECONDS-01 | RFC 6131 §2 | Normative | `:seconds` requires `vacation-seconds`. | `TestVacationSeconds_Capability` |
| SECONDS-02 | RFC 6131 §2, §5.1 | MUST | `vacation-seconds` implies base `vacation`; the latter declaration may be omitted. | `TestVacationSeconds_Capability` |
| SECONDS-03 | RFC 6131 §2 | MUST | Reject simultaneous `:days` and `:seconds`; seconds takes one nonnegative Sieve number. | `TestVacationSeconds_ExclusiveTags` |
| SECONDS-04 | RFC 6131 §2 | MUST | Accept the full valid seconds range without error. | `TestVacationSeconds_Intervals` |
| SECONDS-05 | RFC 6131 §2 | Normative | Preserve second granularity and permit zero as the effective interval. | `TestVacationSeconds_Intervals` |
| SECONDS-06 | RFC 6131 §2 | MAY | Cap excessive intervals at a site maximum of at least 86400 seconds. | `TestVacationSeconds_Intervals` |
| SECONDS-07 | RFC 6131 §2 | Normative | Use the site default when both interval tags are omitted. | `TestVacationSeconds_Intervals` |
| SECONDS-08 | RFC 6131 §3 | Normative | The meeting example requests a 1800-second reply interval. | `TestVacationSeconds_RFCExample` |
| SECONDS-09 | RFC 6131 §2, §4 | SHOULD | Consider reply volume with short intervals. | n/a: host owns sending, persistent suppression and rate limits; interval zero requests no time-based suppression. |
