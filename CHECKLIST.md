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

Not covered yet: the requirements of the extensions the package implements
(RFC 5230 vacation, 5232 imap4flags, 5173 body, 3894 copy, 5429 reject, 5490
mailbox) are each one row in the last section, mapping the existing tests, and
have not been checked clause by clause. The `Received` and loop requirements of
`redirect` belong to the host. One known deviation from the grammar is recorded
as SIEVE-ACT-10.


## Lexical structure (§2.2–2.4)

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-LEX-01 | 2.2 | Normative | Whitespace separates tokens; comments are `#` to the end of the line or bracketed `/* */`, and a bracketed comment must be closed. | `TestSieve_Comments`, `TestRFC5228_SyntaxErrors`, `FuzzParse`, `TestLexer_Errors` |
| SIEVE-LEX-02 | 2.1 | Normative | Command, test and tag names are case-insensitive. | `TestRFC5228_IdentifiersCaseInsensitive`, `FuzzParse` |
| SIEVE-LEX-03 | 2.4.2 | Normative | A quoted string ends at the next unescaped `"`; `\"` and `\\` stand for `"` and `\`; a bare CR, LF or NUL is not allowed inside it, CRLF is. | `TestSieve_EscapedQuoteInString`, `TestLexQuotedString_RejectsBareCRAndNUL`, `TestLexQuotedString_AcceptsCRLF`, `FuzzParse`, `TestLexer_Errors` |
| SIEVE-LEX-04 | 2.4.2 | Normative | A tagged argument is a colon and an identifier; a test or command takes each applicable tag at most once, and a tag it does not define is an error. | `TestValidateSieve_RejectsDuplicateOrInapplicableTags`, `TestValidateSieve_AcceptsSingleTaggedArgs`, `TestSieve_UnknownCommandRejected` |
| SIEVE-LEX-05 | 2.4.2 | Normative | A multi-line string is `text:` and a line break, then lines up to a line holding only `.`; a leading `.` on a line is doubled (dot-stuffing) and the doubling is removed; an unterminated one is an error. | `TestSieve_MultiLineString`, `TestSieve_MultiLineDotStuffing`, `TestRFC5228_SyntaxErrors`, `FuzzParse`, `TestLexer_Errors` |
| SIEVE-LEX-06 | 2.4.2.4 | Normative | With `require "encoded-character"`, `${hex:…}` and `${unicode:…}` in a string stand for the octets or code points they name; text that does not fit the grammar stays as written, a surrogate or code point above U+10FFFF is an error, and the result is not scanned again. | `TestEncodedCharacter_HexDecodesInStringConstant`, `TestEncodedCharacter_HexKeyMatchesHeader`, `TestEncodedCharacter_UnicodeDecodesToUTF8`, `TestEncodedCharacter_MalformedLeftLiteral`, `TestEncodedCharacter_NotRequiredStaysLiteral`, `TestDecodeEncodedCharacters_RFCExamples`, `TestDecodeEncodedCharacters_OutOfRangeUnicode`, `TestEncodedCharacter_InvalidUnicodeIsError`, `FuzzDecodeEncodedCharacters`, `TestEncodedCharacter_SequenceErrors` |
| SIEVE-LEX-07 | 2.4.2.1 | Normative | A string list is one string or `[` strings separated by commas `]`; it holds at least one string, and a comma is followed by a string. | `TestSieve_KeyList`, `TestSieve_HeaderList`, `TestValidateSieve_RejectsEmptyStringList`, `TestRFC5228_SyntaxErrors`, `FuzzParse` |
| SIEVE-LEX-08 | 2.4.2.2 | Normative | A number is digits with an optional quantifier K, M or G (powers of 1024, in either case); a number the implementation cannot represent is an error. | `TestSieve_Size_Quantifier`, `TestValidateSieve_RejectsSizeOverflow`, `TestValidateSieve_AcceptsRepresentableSizes`, `TestRFC5228_SizeTest`, `FuzzParse`, `TestLexer_Errors` |
| SIEVE-LEX-09 | 2.5 | Normative | A block is commands inside `{` and `}`, and may be empty or nested. | `TestSieve_IfElsifElse`, `TestRFC5228_SyntaxErrors` |
| SIEVE-LEX-10 | 2.5 | Normative | A command ends with `;` or, for a control command, with a block; a missing `;`, a missing block and an unbalanced brace are errors. | `TestRFC5228_SyntaxErrors`, `TestValidateSieve_Valid`, `TestValidateSieve_AcceptsNewConstructs`, `FuzzParse` |
| SIEVE-LEX-11 | 2.1, 8.2 | Normative | A command takes exactly the arguments its grammar gives it: `stop`, `discard` and the tests take no extra arguments, and a test list is not empty. | `TestRFC5228_SyntaxErrors` |

## Control commands (§3)

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-CMD-01 | 2.10.5 | MUST | A command or test that is not defined, and not declared with `require`, is an error. | `TestSieve_UnknownCommandRejected`, `TestSieve_UnknownTestRejected`, `TestRequire_RejectsUnknownCommandsAndTests`, `TestRequire_RejectsUndeclaredExtensionUse`, `TestParseSieveScript_RequiresAndCommands`, `TestValidateSieve_Errors` |
| SIEVE-CMD-02 | 3.1 | Normative | `if` runs its block when its test is true; `elsif` and `else` follow only an `if` (or `elsif`), at most one `else`, and only the first branch whose test is true runs. | `TestSieve_IfElsifElse`, `TestSieve_ElsifOnlyFirstMatchRuns`, `TestRFC5228_SyntaxErrors`, `FuzzParse` |
| SIEVE-CMD-03 | 3.2 | MUST | `require` comes before every other command; a capability the implementation does not support is an error; an extension is only usable once declared. Several `require` commands may open a script. | `TestRequire_RejectsUnsupportedExtension`, `TestRequire_RejectsUndeclaredExtensionUse`, `TestRequire_RejectsMisplacedRequire`, `TestRequire_AcceptsProperlyDeclaredScripts`, `TestRequire_ErrorMentionsCapability`, `TestRFC5228_MultipleRequires`, `FuzzParse` |
| SIEVE-CMD-04 | 3.3 | Normative | `stop` ends the script, from any depth of nesting; actions already requested still happen and the implicit keep applies as if the script had ended. | `TestSieve_Stop`, `TestSieve_TopLevelStop`, `TestSieve_StopBeforeActionKeepsImplicitly`, `TestSieve_DiscardThenStopDrops`, `TestRFC5228_StopInNestedBlock` |
| SIEVE-CMD-05 | 10 | MAY | Limit how deeply blocks and test lists nest. | `TestParse_RejectsDeeplyNestedTests`, `TestParse_RejectsDeeplyNestedBlocks`, `TestParse_AcceptsNestingUpToLimit`, `TestParse_WithMaxDepth`, `TestParse_DeeplyNestedDoesNotCrash` |

## Tests (§5)

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-TST-01 | 5.1 | Normative | `address` tests the addresses in the named address headers, with an address part (default `:all`), a comparator and a match type. | `TestSieve_AddressLocalpart`, `TestSieve_AddressDomain`, `TestSieve_AddressAllDefault`, `TestSieve_AddressCc`, `TestSieve_AddressRawHeader_DisplayName`, `TestSieve_AddressRawHeader_AngleAddr`, `TestHeaderTest_StructuredHeaders` |
| SIEVE-TST-02 | 5.1 | Normative | A header value in the named headers that does not parse as an address list contributes no address. | `TestSieve_AddressRawHeader_Unparseable` |
| SIEVE-TST-03 | 5.2 | Normative | `allof` is true when every test in its list is true. | `TestSieve_Allof`, `TestSieve_Allof_OneFalse`, `TestRFC5228_AllofAnyofNested`, `TestRFC5228_SyntaxErrors` |
| SIEVE-TST-04 | 5.3 | Normative | `anyof` is true when any test in its list is true. | `TestSieve_Anyof`, `TestSieve_Anyof_NoneTrue`, `TestRFC5228_AllofAnyofNested` |
| SIEVE-TST-05 | 5.4 | Normative | With `require "envelope"`, `envelope` tests the `from` or `to` part of the SMTP envelope (and any other part the implementation supports) with an address part, comparator and match type; the null reverse-path is the empty string. | `TestSieve_EnvelopeDomainPart`, `TestSieve_EnvelopeFrom_Is`, `TestSieve_EnvelopeTo_Is`, `TestSieve_EnvelopeFrom_Contains`, `TestSieve_EnvelopeFrom_NoMatch`, `TestSieve_EnvelopeFrom_NullReversePath`, `TestSieve_EnvelopeFrom_AbsentDoesNotMatchEmpty`, `TestSieve_EnvelopeFrom_NullReversePath_NoFalseMatch`, `TestSieve_EnvelopeFrom_NullReversePath_Parts`, `TestSieve_EnvelopeFrom_Resolved`, `TestSieve_EnvelopeTo_Resolved`, `TestRFC5228_EnvelopeUnknownPartMatchesNothing` |
| SIEVE-TST-06 | 5.5 | Normative | `exists` is true when every named header is present. | `TestSieve_Exists`, `TestSieve_Exists_AllRequired`, `TestSieve_Exists_RawHeader`, `TestRFC5228_AbsentHeaderNeverMatches`, `TestRFC5228_HeaderNamesCaseInsensitive` |
| SIEVE-TST-07 | 5.6, 5.10 | Normative | `false` is never true and `true` is always true. | `TestSieve_TrueFalse` |
| SIEVE-TST-08 | 5.7 | Normative | `header` is true when any value of any named header matches any key; header names match without regard to case, a header the message lacks matches nothing (not even the empty string), and folded values are unfolded with leading and trailing white space removed. | `TestSieve_HeaderContains`, `TestSieve_HeaderTrimsWhitespace`, `TestSieve_HeaderTrimsWhitespaceContains`, `TestSieve_HeaderUnfoldsContinuation`, `TestSieve_HeaderList`, `TestSieve_KeyList`, `TestRFC5228_AbsentHeaderNeverMatches`, `TestRFC5228_HeaderNamesCaseInsensitive`, `TestRFC5228_SyntaxErrors`, `TestHeaderTest_StructuredHeaders` |
| SIEVE-TST-09 | 5.8 | Normative | `not` inverts its test. | `TestSieve_Not`, `TestSieve_NotAnyofNested`, `TestRFC5228_AllofAnyofNested` |
| SIEVE-TST-10 | 5.9 | Normative | `size :over n` is true when the message is larger than n octets and `:under n` when it is smaller; the size is that of the whole message. | `TestSieve_Size`, `TestSieve_Size_Quantifier`, `TestSieve_Size_IncludesAttachments`, `TestSieve_Size_IncludesHeaders`, `TestSieve_Size_UsesRawSizeWhenSet`, `TestEvaluate_SizeTest_BodyDepthBounded`, `TestRFC5228_SizeTest` |
| SIEVE-TST-11 | 5.9 | Normative | `size` takes exactly one of `:over` and `:under` and a number. | `TestRFC5228_SyntaxErrors` |

## Match types, comparators and address parts (§2.6, §2.7)

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-MT-01 | 2.7.1 | Normative | `:is` requires equal strings, and is the default match type. | `TestRFC5228_MatchTypeIs` |
| SIEVE-MT-02 | 2.7.1 | Normative | `:contains` is true when the key is a substring of the value; the empty key is a substring of every value. | `TestRFC5228_MatchTypeContains` |
| SIEVE-MT-03 | 2.7.1 | Normative | `:matches` takes `*` for any run of characters (including none), `?` for exactly one octet, and `\` to take the next character literally; the whole value must match. | `TestSieve_MatchesStar`, `TestSieve_MatchesQuestionMark`, `TestSieve_MatchesAnchored`, `TestSieve_MatchesEscapedWildcard`, `TestSieve_MatchesQuestionMarkIsOctetNotRune`, `TestRFC5228_MatchesWildcards`, `TestMatchOne_ComparatorsAndRegex` |
| SIEVE-MT-04 | 2.7.1 | Normative | A test takes at most one match type. | `TestValidateSieve_RejectsDuplicateOrInapplicableTags`, `TestRFC5228_SyntaxErrors` |
| SIEVE-MT-05 | 2.7.3 | Normative | `i;ascii-casemap` (the default) compares ASCII letters without regard to case and leaves other octets alone; `i;octet` compares octet for octet. | `TestSieve_ComparatorCasemapDefault`, `TestSieve_ComparatorOctetCaseSensitive`, `TestSieve_ComparatorAsciiCasemapAsciiOnly`, `TestComparator_ValidCombinationsStillParse`, `TestMatchOne_ComparatorsAndRegex` |
| SIEVE-MT-06 | 2.7.3 | MUST | A comparator the implementation does not support is an error, whether or not the script declares it. | `TestComparator_UnknownRejected`, `TestRFC5228_SyntaxErrors` |
| SIEVE-MT-07 | 2.7.3, RFC 4790 §9.1.1 | Normative | `i;ascii-numeric` (declared with `require "comparator-i;ascii-numeric"`) compares leading digits as numbers and supports only `:is`; `:contains` and `:matches` are errors. | `TestSieve_ComparatorAsciiNumeric`, `TestComparator_AsciiNumericRejectsSubstringMatchTypes`, `TestComparator_AsciiNumericIsSemantics`, `TestComparator_ValidCombinationsStillParse` |
| SIEVE-MT-08 | 2.7.4 | Normative | `:localpart`, `:domain` and `:all` pick the part of an address that is compared; a test takes at most one. | `TestSieve_AddressLocalpart`, `TestSieve_AddressDomain`, `TestSieve_AddressAllDefault`, `TestSieve_EnvelopeDomainPart`, `TestRFC5228_SyntaxErrors` |

## Actions (§4) and their effect (§2.10)

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-ACT-01 | 4.1 | Normative | `keep` files the message into the default mailbox and cancels the implicit keep. | `TestSieve_DuplicateKeepDeliversOnce`, `TestRFC5228_SyntaxErrors` |
| SIEVE-ACT-02 | 4.1 | Normative | With `require "fileinto"`, `fileinto` files the message into the named mailbox and cancels the implicit keep; filing into one mailbox twice delivers once. | `TestSieve_DuplicateFileintoDeliversOnce`, `TestSieve_DistinctFileintoDeliversEach`, `TestSieve_FileintoCopyDistinctFromConsuming`, `TestSieve_FileintoThenDiscardStillDelivers`, `TestSieve_DiscardThenFileintoStillDelivers` |
| SIEVE-ACT-03 | 4.2 | Normative | `redirect` sends the message to one address and cancels the implicit keep; an address with control characters is an error; redirecting to one address twice delivers once. | `TestSieve_Redirect`, `TestSieve_DuplicateRedirectDeliversOnce`, `TestSieve_DistinctRedirectDeliversEach`, `TestValidateSieve_RejectsRedirectControlChars`, `TestValidateSieve_AcceptsNormalRedirect`, `TestRFC5228_SyntaxErrors` |
| SIEVE-ACT-04 | 4.3 | SHOULD | An implementation keeps a redirect from looping and adds a `Received` or similar marker. | n/a: the library only reports the redirect to the host's Executor; loop prevention and the message's headers are the host's. |
| SIEVE-ACT-05 | 4.4 | Normative | `discard` cancels the implicit keep, drops the message if nothing else delivers it, and does not end the script or withdraw other actions. | `TestSieve_Discard`, `TestSieve_BareDiscardDropsMessage`, `TestSieve_DiscardDoesNotStopScript`, `TestSieve_DiscardThenFileintoStillDelivers`, `TestSieve_FileintoThenDiscardStillDelivers`, `TestSieve_DiscardThenStopDrops` |
| SIEVE-ACT-06 | 2.10.2 | MUST | Unless an action cancels it, the implicit keep delivers the message to the default mailbox, also for an empty script and a script that selects no action. | `TestSieve_NoActionImpliesImplicitKeep`, `TestSieve_EmptyScriptImpliesImplicitKeep`, `TestSieve_StopBeforeActionKeepsImplicitly`, `TestSieve_CopyImplicitKeep`, `TestSieve_RedirectCopyTag` |
| SIEVE-ACT-07 | 2.10.6 | MUST | When an error occurs while the script runs, the message is kept rather than lost. | `TestSieve_RuntimeErrorKeepsMessage`, `TestEvaluate_NilMessage_DoesNotPanic`, `TestEvaluate_NilMessage_NilScriptStillGuarded`, `TestEvaluate_PanicWithErrorValue` |
| SIEVE-ACT-08 | 2.10.4 | Normative | Actions run in script order, each tests the message as received (actions do not change it), and a test that cannot match does not stop the script. | `TestSieve_CombinedExtensions`, `TestSieve_InvalidRegex_Skipped` |
| SIEVE-ACT-09 | 6.1 | Normative | `fileinto`, `reject`, `envelope`, `vacation` and the other extensions are only usable once declared, each under its capability string. | `TestRequire_AcceptsProperlyDeclaredScripts`, `TestRequire_RejectsUndeclaredExtensionUse` |
| SIEVE-ACT-10 | 4.1, 8.2 | Normative | `keep` takes no positional arguments of its own. | n/a: a known gap. The parser skips any arguments of `keep` so that `keep :flags …` (imap4flags) loads; `keep "x";` is accepted although the grammar of RFC 5228 §8.2 has no argument for it. Tracked in the PR description. |

## Beyond RFC 5228: the extensions the package implements

The package also implements extensions that RFC 5228 only allows. Each has a row below mapping its existing tests; checking each extension's own RFC requirement by requirement (RFC 5230, 5232, 5173, 3894, 5429, 5490) is not done yet and is listed as a gap in the pull request.

| ID | Section | Level | Requirement | Tests |
|----|---------|-------|-------------|-------|
| SIEVE-EXT-01 | RFC 5230 | Normative | `vacation` (with `:days`, `:seconds`, `:subject`, a reason) asks the host to send an auto-reply to the envelope sender; it is suppressed for the null reverse-path. | `TestSieve_Vacation_Basic`, `TestSieve_Vacation_DefaultDays`, `TestSieve_Vacation_Conditional`, `TestSieve_Vacation_UsesEnvelopeSender`, `TestSieve_Vacation_MultiLine`, `TestSieve_Vacation_SecondsGranularity`, `TestSieve_Vacation_NullReversePathSuppressed`, `TestVacation_ReplyTo`, `TestExtensionArguments_Parse` |
| SIEVE-EXT-02 | RFC 5232 | Normative | `setflag`, `addflag` and `removeflag` change the flag set that later `keep` and `fileinto` use. | `TestSieve_SetAndAddFlag`, `TestSieve_RemoveFlag`, `TestSieve_SetFlagEscaped`, `TestExtensionArguments_Parse` |
| SIEVE-EXT-03 | RFC 5173 | Normative | `body` tests the text of the message body (HTML tags stripped, the plain part of a multipart preferred) with a match type and comparator. | `TestSieve_BodyContains`, `TestSieve_BodyContains_NoMatch`, `TestSieve_BodyContains_HTMLStripped`, `TestSieve_BodyContains_MultipartPlainPreferred`, `TestSieve_BodyIs`, `TestExtractBodyText_PlainText`, `TestExtractBodyText_HTML`, `TestExtractBodyText_Multipart`, `TestExtractBodyText_MultipartHTMLOnly`, `TestStripHTMLTags`, `TestEvaluate_BodyTest_DepthBounded`, `TestEvaluate_BodyTest_DepthWithinCapStillMatches`, `TestEvaluate_BodyTest_PartCountBounded` |
| SIEVE-EXT-04 | RFC 3894 | Normative | `:copy` on `fileinto` and `redirect` leaves the implicit keep in effect. | `TestSieve_RedirectCopyTag`, `TestSieve_CopyImplicitKeep`, `TestSieve_FileintoCopyDistinctFromConsuming`, `TestExtensionArguments_Parse` |
| SIEVE-EXT-05 | RFC 5429, 5490 | Normative | `reject` refuses the message with a reason; `fileinto :create` asks the host to create the mailbox. | `TestSieve_Reject`, `TestSieve_FileintoCreate`, `TestExtensionArguments_Parse` |
| SIEVE-EXT-06 | non-standard | Normative | `:regex` is a match type of the package, not of any RFC; an invalid expression never matches. | `TestSieve_HeaderRegex`, `TestSieve_HeaderRegex_NoMatch`, `TestSieve_HeaderRegex_CaseInsensitive`, `TestSieve_BodyRegex`, `TestSieve_AddressRegex`, `TestSieve_EnvelopeRegex`, `TestMatchOne_ComparatorsAndRegex` |
| SIEVE-EXT-07 | non-standard | Normative | `notify` asks the host to send a notification. | `TestSieve_Notify_Basic`, `TestSieve_Notify_Conditional`, `TestSieve_Notify_NoMatch`, `TestSieve_Notify_MultiLine`, `TestExtensionArguments_Parse` |
