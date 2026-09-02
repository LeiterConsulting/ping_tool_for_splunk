package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// parsePSD1File parses the constant-data subset used by Ping Monitor PSD1
// configuration. It never executes PowerShell or expands expressions.
func parsePSD1File(path string) (map[string]interface{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parsePSD1(f)
}

func parsePSD1(r io.Reader) (map[string]interface{}, error) {
	source, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("psd1: read: %w", err)
	}
	p := &psd1Parser{s: newPSD1Scanner(source)}
	value, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	result, ok := value.(map[string]interface{})
	if !ok {
		return nil, p.errorAt(p.s.peek(), "expected top-level hashtable, got %T", value)
	}
	if token := p.s.peek(); token.kind != psd1TokEOF {
		return nil, p.errorAt(token, "unexpected trailing token %q", token.text)
	}
	return result, nil
}

type psd1TokenKind int

const (
	psd1TokEOF psd1TokenKind = iota
	psd1TokInvalid
	psd1TokIdent
	psd1TokString
	psd1TokNumber
	psd1TokBool
	psd1TokNull
	psd1TokAt
	psd1TokLBrace
	psd1TokRBrace
	psd1TokLParen
	psd1TokRParen
	psd1TokEq
	psd1TokComma
	psd1TokSemi
)

type psd1Token struct {
	kind   psd1TokenKind
	text   string
	line   int
	column int
	err    error
}

type psd1Scanner struct {
	source  []rune
	index   int
	line    int
	column  int
	peeked  bool
	peekTok psd1Token
}

func newPSD1Scanner(source []byte) *psd1Scanner {
	return &psd1Scanner{source: []rune(string(source)), line: 1, column: 1}
}

func (s *psd1Scanner) peek() psd1Token {
	if !s.peeked {
		s.peekTok = s.next()
		s.peeked = true
	}
	return s.peekTok
}

func (s *psd1Scanner) consume() psd1Token {
	if s.peeked {
		s.peeked = false
		return s.peekTok
	}
	return s.next()
}

func (s *psd1Scanner) next() psd1Token {
	if err := s.skipSpaceAndComments(); err != nil {
		return s.invalidToken(err)
	}
	startLine, startColumn := s.line, s.column
	character, ok := s.advance()
	if !ok {
		return psd1Token{kind: psd1TokEOF, line: startLine, column: startColumn}
	}

	simple := func(kind psd1TokenKind) psd1Token {
		return psd1Token{kind: kind, text: string(character), line: startLine, column: startColumn}
	}
	switch character {
	case '@':
		return simple(psd1TokAt)
	case '{':
		return simple(psd1TokLBrace)
	case '}':
		return simple(psd1TokRBrace)
	case '(':
		return simple(psd1TokLParen)
	case ')':
		return simple(psd1TokRParen)
	case '=':
		return simple(psd1TokEq)
	case ',':
		return simple(psd1TokComma)
	case ';':
		return simple(psd1TokSemi)
	case '\ufeff':
		return s.next()
	case '\'', '"':
		value, err := s.readString(character)
		if err != nil {
			return psd1Token{kind: psd1TokInvalid, line: startLine, column: startColumn, err: err}
		}
		return psd1Token{kind: psd1TokString, text: value, line: startLine, column: startColumn}
	default:
		if isWordStart(character) {
			return s.readWord(character, startLine, startColumn)
		}
		return psd1Token{
			kind: psd1TokInvalid, text: string(character), line: startLine, column: startColumn,
			err: fmt.Errorf("unsupported character %q", character),
		}
	}
}

func (s *psd1Scanner) skipSpaceAndComments() error {
	for {
		character, ok := s.peekRune(0)
		if !ok {
			return nil
		}
		if unicode.IsSpace(character) || character == '\ufeff' {
			s.advance()
			continue
		}
		if character == '#' {
			for {
				current, exists := s.advance()
				if !exists || current == '\n' {
					break
				}
			}
			continue
		}
		if character == '<' {
			next, exists := s.peekRune(1)
			if exists && next == '#' {
				startLine, startColumn := s.line, s.column
				s.advance()
				s.advance()
				for {
					current, exists := s.peekRune(0)
					if !exists {
						return fmt.Errorf("unterminated block comment beginning at line %d, column %d", startLine, startColumn)
					}
					after, hasAfter := s.peekRune(1)
					if current == '#' && hasAfter && after == '>' {
						s.advance()
						s.advance()
						break
					}
					s.advance()
				}
				continue
			}
		}
		return nil
	}
}

func (s *psd1Scanner) readString(quote rune) (string, error) {
	var result bytes.Buffer
	for {
		character, ok := s.advance()
		if !ok {
			return "", fmt.Errorf("unterminated string")
		}
		if quote == '\'' {
			if character != '\'' {
				result.WriteRune(character)
				continue
			}
			if next, exists := s.peekRune(0); exists && next == '\'' {
				s.advance()
				result.WriteRune('\'')
				continue
			}
			return result.String(), nil
		}

		if character == '"' {
			return result.String(), nil
		}
		if character != '`' {
			result.WriteRune(character)
			continue
		}
		escaped, exists := s.advance()
		if !exists {
			return "", fmt.Errorf("unterminated escape")
		}
		switch escaped {
		case '0':
			result.WriteByte(0)
		case 'a':
			result.WriteByte('\a')
		case 'b':
			result.WriteByte('\b')
		case 'e':
			result.WriteByte(0x1b)
		case 'f':
			result.WriteByte('\f')
		case 'n':
			result.WriteByte('\n')
		case 'r':
			result.WriteByte('\r')
		case 't':
			result.WriteByte('\t')
		case 'v':
			result.WriteByte('\v')
		case '\r':
			if next, ok := s.peekRune(0); ok && next == '\n' {
				s.advance()
			}
		case '\n':
			// PowerShell line continuation.
		default:
			result.WriteRune(escaped)
		}
	}
}

func (s *psd1Scanner) readWord(first rune, line int, column int) psd1Token {
	var result bytes.Buffer
	result.WriteRune(first)
	for {
		character, ok := s.peekRune(0)
		if !ok || unicode.IsSpace(character) || strings.ContainsRune("{}()=,;#", character) {
			break
		}
		s.advance()
		result.WriteRune(character)
	}
	text := result.String()
	switch strings.ToLower(text) {
	case "$true":
		return psd1Token{kind: psd1TokBool, text: "true", line: line, column: column}
	case "$false":
		return psd1Token{kind: psd1TokBool, text: "false", line: line, column: column}
	case "$null":
		return psd1Token{kind: psd1TokNull, text: "null", line: line, column: column}
	}
	if looksNumeric(text) {
		value, err := strconv.ParseInt(text, 10, 0)
		if err != nil {
			return psd1Token{kind: psd1TokInvalid, text: text, line: line, column: column, err: fmt.Errorf("invalid integer %q: %w", text, err)}
		}
		return psd1Token{kind: psd1TokNumber, text: strconv.FormatInt(value, 10), line: line, column: column}
	}
	return psd1Token{kind: psd1TokIdent, text: text, line: line, column: column}
}

func (s *psd1Scanner) invalidToken(err error) psd1Token {
	return psd1Token{kind: psd1TokInvalid, line: s.line, column: s.column, err: err}
}

func (s *psd1Scanner) peekRune(offset int) (rune, bool) {
	index := s.index + offset
	if index < 0 || index >= len(s.source) {
		return 0, false
	}
	return s.source[index], true
}

func (s *psd1Scanner) advance() (rune, bool) {
	if s.index >= len(s.source) {
		return 0, false
	}
	character := s.source[s.index]
	s.index++
	if character == '\n' {
		s.line++
		s.column = 1
	} else {
		s.column++
	}
	return character, true
}

func isWordStart(character rune) bool {
	return unicode.IsLetter(character) || unicode.IsDigit(character) || strings.ContainsRune("_$.-\\/", character)
}

func looksNumeric(value string) bool {
	if value == "" {
		return false
	}
	if value[0] >= '0' && value[0] <= '9' {
		return true
	}
	return len(value) > 1 && value[0] == '-' && value[1] >= '0' && value[1] <= '9'
}

type psd1Parser struct {
	s *psd1Scanner
}

func (p *psd1Parser) errorAt(token psd1Token, format string, args ...interface{}) error {
	message := fmt.Sprintf(format, args...)
	if token.err != nil {
		message = token.err.Error()
	}
	return fmt.Errorf("psd1:%d:%d: %s", max(1, token.line), max(1, token.column), message)
}

func (p *psd1Parser) expect(kind psd1TokenKind) (psd1Token, error) {
	token := p.s.consume()
	if token.kind == psd1TokInvalid {
		return psd1Token{}, p.errorAt(token, "invalid token")
	}
	if token.kind != kind {
		return psd1Token{}, p.errorAt(token, "expected %s, got %q", tokenKindName(kind), token.text)
	}
	return token, nil
}

func (p *psd1Parser) parseValue() (interface{}, error) {
	token := p.s.peek()
	if token.kind == psd1TokInvalid {
		return nil, p.errorAt(token, "invalid token")
	}
	switch token.kind {
	case psd1TokAt:
		p.s.consume()
		next := p.s.peek()
		switch next.kind {
		case psd1TokLBrace:
			return p.parseHashtable()
		case psd1TokLParen:
			return p.parseArray()
		default:
			return nil, p.errorAt(next, "expected { or ( after @, got %q", next.text)
		}
	case psd1TokString:
		return p.s.consume().text, nil
	case psd1TokNumber:
		value, err := strconv.ParseInt(p.s.consume().text, 10, 0)
		if err != nil {
			return nil, p.errorAt(token, "invalid integer %q", token.text)
		}
		return int(value), nil
	case psd1TokBool:
		return strings.EqualFold(p.s.consume().text, "true"), nil
	case psd1TokNull:
		p.s.consume()
		return nil, nil
	case psd1TokIdent:
		return p.s.consume().text, nil
	case psd1TokEOF:
		return nil, p.errorAt(token, "unexpected end of file")
	default:
		return nil, p.errorAt(token, "unexpected token %q", token.text)
	}
}

func (p *psd1Parser) parseHashtable() (map[string]interface{}, error) {
	if _, err := p.expect(psd1TokLBrace); err != nil {
		return nil, err
	}
	result := make(map[string]interface{})
	seen := make(map[string]psd1Token)
	for {
		token := p.s.peek()
		if token.kind == psd1TokInvalid {
			return nil, p.errorAt(token, "invalid token")
		}
		switch token.kind {
		case psd1TokRBrace:
			p.s.consume()
			return result, nil
		case psd1TokSemi, psd1TokComma:
			p.s.consume()
			continue
		case psd1TokEOF:
			return nil, p.errorAt(token, "unexpected end of file in hashtable")
		}

		keyToken := p.s.consume()
		if keyToken.kind != psd1TokIdent && keyToken.kind != psd1TokString {
			return nil, p.errorAt(keyToken, "expected key, got %q", keyToken.text)
		}
		key := strings.TrimSpace(keyToken.text)
		if key == "" {
			return nil, p.errorAt(keyToken, "hashtable key must not be empty")
		}
		lookupKey := strings.ToLower(key)
		if original, exists := seen[lookupKey]; exists {
			return nil, p.errorAt(keyToken, "duplicate key %q; first declared at line %d, column %d", key, original.line, original.column)
		}
		if _, err := p.expect(psd1TokEq); err != nil {
			return nil, err
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		seen[lookupKey] = keyToken
		result[key] = value
	}
}

func (p *psd1Parser) parseArray() ([]interface{}, error) {
	if _, err := p.expect(psd1TokLParen); err != nil {
		return nil, err
	}
	items := make([]interface{}, 0, 4)
	for {
		token := p.s.peek()
		if token.kind == psd1TokInvalid {
			return nil, p.errorAt(token, "invalid token")
		}
		switch token.kind {
		case psd1TokRParen:
			p.s.consume()
			return items, nil
		case psd1TokComma, psd1TokSemi:
			p.s.consume()
			continue
		case psd1TokEOF:
			return nil, p.errorAt(token, "unexpected end of file in array")
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		items = append(items, value)
	}
}

func tokenKindName(kind psd1TokenKind) string {
	switch kind {
	case psd1TokLBrace:
		return "{"
	case psd1TokRBrace:
		return "}"
	case psd1TokLParen:
		return "("
	case psd1TokRParen:
		return ")"
	case psd1TokEq:
		return "="
	default:
		return fmt.Sprintf("token %d", kind)
	}
}
