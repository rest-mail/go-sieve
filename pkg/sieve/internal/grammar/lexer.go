package grammar

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ── Tokenizer ────────────────────────────────────────────────────────

// Kind identifies a lexical token.
type Kind int

const (
	EOF Kind = iota
	Ident
	Tag
	String
	Number
	LBrace
	RBrace
	LParen
	RParen
	LBracket
	RBracket
	Comma
	Semicolon
)

// Token carries one source token and its starting line.
type Token struct {
	Kind Kind
	Str  string // ident/tag name (lowercased) or string value
	Num  int64  // for Number
	Line int
}

// Lexer scans source without interpreting commands or tests.
type Lexer struct {
	src  string
	Pos  int
	line int
}

// New starts a lexer at the first line of src.
func New(src string) *Lexer { return &Lexer{src: src, line: 1} }

func (l *Lexer) errf(format string, args ...interface{}) error {
	return fmt.Errorf("sieve: line %d: %s", l.line, fmt.Sprintf(format, args...))
}

// Tokenize scans the whole source into a token slice terminated by EOF.
func (l *Lexer) Tokenize() ([]Token, error) {
	// RFC 5228 §2.1 forbids raw NUL everywhere, including comments and
	// multiline strings. Check source before skipping or decoding any of it;
	// encoded-character may still produce NUL in a string value (§2.4.2.4).
	if i := strings.IndexByte(l.src, 0); i >= 0 {
		return nil, fmt.Errorf("sieve: line %d: NUL byte in script", 1+strings.Count(l.src[:i], "\n"))
	}
	// RFC 5228 §8.1 permits CR only in a CRLF pair, in every lexical
	// context. Validate before comments or multiline strings can hide it.
	// Encoded-character expansion happens later and may produce a bare CR.
	for i := 0; i < len(l.src); i++ {
		if l.src[i] == '\r' && (i+1 == len(l.src) || l.src[i+1] != '\n') {
			return nil, fmt.Errorf("sieve: line %d: bare CR in script", 1+strings.Count(l.src[:i], "\n"))
		}
	}
	var toks []Token
	for {
		if err := l.skipTrivia(); err != nil {
			return nil, err
		}
		if l.Pos >= len(l.src) {
			toks = append(toks, Token{Kind: EOF, Line: l.line})
			return toks, nil
		}

		// Multi-line string: "text:" at a token boundary, followed only by
		// optional whitespace / comment until end of line.
		if l.atMultiLineString() {
			s, err := l.readMultiLineString()
			if err != nil {
				return nil, err
			}
			toks = append(toks, Token{Kind: String, Str: s, Line: l.line})
			continue
		}

		c := l.src[l.Pos]
		switch {
		case c == '{':
			toks = append(toks, Token{Kind: LBrace, Line: l.line})
			l.Pos++
		case c == '}':
			toks = append(toks, Token{Kind: RBrace, Line: l.line})
			l.Pos++
		case c == '(':
			toks = append(toks, Token{Kind: LParen, Line: l.line})
			l.Pos++
		case c == ')':
			toks = append(toks, Token{Kind: RParen, Line: l.line})
			l.Pos++
		case c == '[':
			toks = append(toks, Token{Kind: LBracket, Line: l.line})
			l.Pos++
		case c == ']':
			toks = append(toks, Token{Kind: RBracket, Line: l.line})
			l.Pos++
		case c == ',':
			toks = append(toks, Token{Kind: Comma, Line: l.line})
			l.Pos++
		case c == ';':
			toks = append(toks, Token{Kind: Semicolon, Line: l.line})
			l.Pos++
		case c == '"':
			startLine := l.line // a CRLF inside the string advances l.line
			s, err := l.readQuotedString()
			if err != nil {
				return nil, err
			}
			toks = append(toks, Token{Kind: String, Str: s, Line: startLine})
		case c == ':':
			tag := l.readTag()
			toks = append(toks, Token{Kind: Tag, Str: tag, Line: l.line})
		case c >= '0' && c <= '9':
			n, err := l.ReadNumber()
			if err != nil {
				return nil, err
			}
			toks = append(toks, Token{Kind: Number, Num: n, Line: l.line})
		case isIdentStart(c):
			toks = append(toks, Token{Kind: Ident, Str: strings.ToLower(l.readIdent()), Line: l.line})
		default:
			return nil, l.errf("unexpected character %q", string(c))
		}
	}
}

// skipTrivia advances over whitespace, hash comments and bracket comments.
func (l *Lexer) skipTrivia() error {
	for l.Pos < len(l.src) {
		c := l.src[l.Pos]
		switch {
		case c == '\n':
			l.line++
			l.Pos++
		case c == ' ' || c == '\t' || c == '\r':
			l.Pos++
		case c == '#':
			for l.Pos < len(l.src) && l.src[l.Pos] != '\n' {
				l.Pos++
			}
		case c == '/' && l.Pos+1 < len(l.src) && l.src[l.Pos+1] == '*':
			l.Pos += 2
			for {
				if l.Pos+1 >= len(l.src) {
					return l.errf("unterminated bracket comment")
				}
				if l.src[l.Pos] == '*' && l.src[l.Pos+1] == '/' {
					l.Pos += 2
					break
				}
				if l.src[l.Pos] == '\n' {
					l.line++
				}
				l.Pos++
			}
		default:
			return nil
		}
	}
	return nil
}

func (l *Lexer) readQuotedString() (string, error) {
	l.Pos++ // opening quote
	var b strings.Builder
	for l.Pos < len(l.src) {
		c := l.src[l.Pos]
		switch c {
		case '"':
			l.Pos++
			return b.String(), nil
		case '\\':
			// RFC 5228 §8.1: quoted-special and quoted-other remove the
			// backslash, but neither permits NUL, CR or LF after it.
			if l.Pos+1 < len(l.src) {
				switch l.src[l.Pos+1] {
				case '\r', '\n':
					return "", l.errf("invalid escaped octet in quoted string")
				}
				l.Pos++
				b.WriteByte(l.src[l.Pos])
				l.Pos++
				continue
			}
			l.Pos++
		case '\r':
			// Tokenize has already checked that every CR belongs to CRLF.
			// Preserve the pair in the value (RFC 5228 §8.1 quoted-safe).
			b.WriteString("\r\n")
			l.Pos += 2
			l.line++
		case '\n':
			// A lone LF (not part of a CRLF) is not a valid quoted-char.
			return "", l.errf("bare LF in quoted string")
		default:
			b.WriteByte(c)
			l.Pos++
		}
	}
	return "", l.errf("unterminated string")
}

func (l *Lexer) readTag() string {
	start := l.Pos
	l.Pos++ // colon
	for l.Pos < len(l.src) && isIdentChar(l.src[l.Pos]) {
		l.Pos++
	}
	return strings.ToLower(l.src[start:l.Pos])
}

func (l *Lexer) readIdent() string {
	start := l.Pos
	for l.Pos < len(l.src) && isIdentChar(l.src[l.Pos]) {
		l.Pos++
	}
	return l.src[start:l.Pos]
}

// ReadNumber reads a non-negative integer with an optional K/M/G quantifier
// (RFC 5228 §2.4.1). K=1024, M=1024^2, G=1024^3.
func (l *Lexer) ReadNumber() (int64, error) {
	start := l.Pos
	for l.Pos < len(l.src) && l.src[l.Pos] >= '0' && l.src[l.Pos] <= '9' {
		l.Pos++
	}
	digits := l.src[start:l.Pos]
	n, err := strconv.ParseInt(digits, 10, 64)
	if err != nil {
		return 0, l.errf("invalid number %q", digits)
	}
	if l.Pos < len(l.src) {
		var factor int64
		switch l.src[l.Pos] {
		case 'K', 'k':
			factor = 1024
		case 'M', 'm':
			factor = 1024 * 1024
		case 'G', 'g':
			factor = 1024 * 1024 * 1024
		}
		if factor != 0 {
			// Guard the quantifier multiply: n is non-negative, so an overflow
			// would wrap to a negative (or truncated) limit and silently invert
			// a size test. RFC 5228 §2.4.1 makes an unrepresentable number a
			// syntax error, so reject it rather than wrap.
			if n > math.MaxInt64/factor {
				lit := l.src[start : l.Pos+1] // digits plus the quantifier
				return 0, l.errf("number %q overflows int64", lit)
			}
			n *= factor
			l.Pos++
		}
	}
	return n, nil
}

// atMultiLineString reports whether the cursor sits on a "text:" multi-line
// string token: the literal "text:" followed by only whitespace and an
// optional hash comment up to the end of the line.
func (l *Lexer) atMultiLineString() bool {
	const marker = "text:"
	if l.Pos+len(marker) > len(l.src) {
		return false
	}
	if !strings.EqualFold(l.src[l.Pos:l.Pos+len(marker)], marker) {
		return false
	}
	i := l.Pos + len(marker)
	for i < len(l.src) && (l.src[i] == ' ' || l.src[i] == '\t' || l.src[i] == '\r') {
		i++
	}
	if i < len(l.src) && l.src[i] == '#' {
		for i < len(l.src) && l.src[i] != '\n' {
			i++
		}
	}
	return i >= len(l.src) || l.src[i] == '\n'
}

// readMultiLineString consumes a "text:" block up to a line containing only a
// single period, applying dot-unstuffing (a leading ".." becomes ".").
func (l *Lexer) readMultiLineString() (string, error) {
	// Skip "text:" and the remainder of the introducing line.
	l.Pos += len("text:")
	for l.Pos < len(l.src) && l.src[l.Pos] != '\n' {
		l.Pos++
	}
	if l.Pos < len(l.src) {
		l.Pos++ // consume newline
		l.line++
	}

	var b strings.Builder
	for {
		if l.Pos >= len(l.src) {
			return "", l.errf("unterminated multi-line string")
		}
		lineStart := l.Pos
		for l.Pos < len(l.src) && l.src[l.Pos] != '\n' {
			l.Pos++
		}
		line := l.src[lineStart:l.Pos]
		ending := "\n"
		if strings.HasSuffix(line, "\r") {
			line = strings.TrimSuffix(line, "\r")
			ending = "\r\n"
		}
		if l.Pos < len(l.src) {
			l.Pos++ // newline
			l.line++
		}
		if line == "." {
			return b.String(), nil
		}
		if strings.HasPrefix(line, "..") {
			line = line[1:] // Only doubled leading dots are unstuffed (§2.4.2).
		}
		b.WriteString(line)
		b.WriteString(ending)
	}
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '-' || c == '.' || c == '_'
}
