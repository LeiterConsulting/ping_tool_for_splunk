package config

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
)

// parsePSD1File parses a PowerShell data file (.psd1) containing a hashtable literal.
// This is a small, non-executing parser intended for our config.psd1 shape.
// Supported value types:
// - strings: 'single quoted' (” escape) and "double quoted" (basic `-escapes)
// - numbers: integers (also treated as int)
// - booleans: $true/$false
// - null: $null
// - nested hashtables: @{ key = value; ... }
// - simple arrays: @(a, b, @{...}) (returned as []interface{})
func parsePSD1File(path string) (map[string]interface{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parsePSD1(f)
}

func parsePSD1(r io.Reader) (map[string]interface{}, error) {
	p := &psd1Parser{s: newPSD1Scanner(r)}
	val, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	m, ok := val.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("psd1: expected top-level hashtable, got %T", val)
	}
	if tok := p.s.peek(); tok.kind != psd1TokEOF {
		return nil, fmt.Errorf("psd1: unexpected trailing token %q", tok.text)
	}
	return m, nil
}

type psd1TokenKind int

const (
	psd1TokEOF psd1TokenKind = iota
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
	kind psd1TokenKind
	text string
}

type psd1Scanner struct {
	r       *bufio.Reader
	peeked  bool
	peekTok psd1Token
}

func newPSD1Scanner(r io.Reader) *psd1Scanner {
	return &psd1Scanner{r: bufio.NewReader(r)}
}

func (s *psd1Scanner) peek() psd1Token {
	if s.peeked {
		return s.peekTok
	}
	s.peekTok = s.next()
	s.peeked = true
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
	s.skipSpaceAndComments()
	ch, _, err := s.r.ReadRune()
	if err != nil {
		return psd1Token{kind: psd1TokEOF}
	}

	switch ch {
	case '@':
		return psd1Token{kind: psd1TokAt, text: "@"}
	case '{':
		return psd1Token{kind: psd1TokLBrace, text: "{"}
	case '}':
		return psd1Token{kind: psd1TokRBrace, text: "}"}
	case '(':
		return psd1Token{kind: psd1TokLParen, text: "("}
	case ')':
		return psd1Token{kind: psd1TokRParen, text: ")"}
	case '=':
		return psd1Token{kind: psd1TokEq, text: "="}
	case ',':
		return psd1Token{kind: psd1TokComma, text: ","}
	case ';':
		return psd1Token{kind: psd1TokSemi, text: ";"}
	case '\ufeff':
		// BOM; continue.
		return s.next()
	case '\'', '"':
		str, err := s.readString(ch)
		if err != nil {
			return psd1Token{kind: psd1TokEOF}
		}
		return psd1Token{kind: psd1TokString, text: str}
	default:
		if isIdentStart(ch) || ch == '$' || unicode.IsDigit(ch) || ch == '-' {
			_ = s.r.UnreadRune()
			return s.readWord()
		}
		return psd1Token{kind: psd1TokIdent, text: string(ch)}
	}
}

func (s *psd1Scanner) skipSpaceAndComments() {
	for {
		ch, _, err := s.r.ReadRune()
		if err != nil {
			return
		}
		if unicode.IsSpace(ch) {
			continue
		}
		if ch == '#' {
			// Comment to end of line.
			for {
				c, _, e := s.r.ReadRune()
				if e != nil || c == '\n' {
					break
				}
			}
			continue
		}
		_ = s.r.UnreadRune()
		return
	}
}

func (s *psd1Scanner) readString(quote rune) (string, error) {
	var b bytes.Buffer
	for {
		ch, _, err := s.r.ReadRune()
		if err != nil {
			return "", fmt.Errorf("psd1: unterminated string")
		}
		if quote == '\'' {
			if ch == '\'' {
				// In single-quoted PowerShell strings, '' is an escaped single quote.
				next, _, e := s.r.ReadRune()
				if e == nil && next == '\'' {
					b.WriteRune('\'')
					continue
				}
				if e == nil {
					_ = s.r.UnreadRune()
				}
				break
			}
			b.WriteRune(ch)
			continue
		}

		// Double-quoted: implement minimal backtick escaping.
		if ch == '"' {
			break
		}
		if ch == '`' {
			n, _, e := s.r.ReadRune()
			if e != nil {
				return "", fmt.Errorf("psd1: unterminated escape")
			}
			b.WriteRune(n)
			continue
		}
		b.WriteRune(ch)
	}
	return b.String(), nil
}

func (s *psd1Scanner) readWord() psd1Token {
	var b bytes.Buffer
	for {
		ch, _, err := s.r.ReadRune()
		if err != nil {
			break
		}
		if unicode.IsSpace(ch) || strings.ContainsRune("{}()=,;", ch) || ch == '#' {
			_ = s.r.UnreadRune()
			break
		}
		b.WriteRune(ch)
	}
	text := b.String()
	low := strings.ToLower(text)
	if low == "$true" {
		return psd1Token{kind: psd1TokBool, text: "true"}
	}
	if low == "$false" {
		return psd1Token{kind: psd1TokBool, text: "false"}
	}
	if low == "$null" {
		return psd1Token{kind: psd1TokNull, text: "null"}
	}
	if n, err := strconv.Atoi(text); err == nil {
		return psd1Token{kind: psd1TokNumber, text: strconv.Itoa(n)}
	}
	return psd1Token{kind: psd1TokIdent, text: text}
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_' || r == '.'
}

type psd1Parser struct {
	s *psd1Scanner
}

func (p *psd1Parser) expect(kind psd1TokenKind) (psd1Token, error) {
	tok := p.s.consume()
	if tok.kind != kind {
		return psd1Token{}, fmt.Errorf("psd1: expected %v, got %q", kind, tok.text)
	}
	return tok, nil
}

func (p *psd1Parser) parseValue() (interface{}, error) {
	tok := p.s.peek()
	switch tok.kind {
	case psd1TokAt:
		_ = p.s.consume()
		n := p.s.peek()
		switch n.kind {
		case psd1TokLBrace:
			return p.parseHashtable()
		case psd1TokLParen:
			return p.parseArray()
		default:
			return nil, fmt.Errorf("psd1: unexpected token after @: %q", n.text)
		}
	case psd1TokString:
		return p.s.consume().text, nil
	case psd1TokNumber:
		n, _ := strconv.Atoi(p.s.consume().text)
		return n, nil
	case psd1TokBool:
		b := strings.EqualFold(p.s.consume().text, "true")
		return b, nil
	case psd1TokNull:
		_ = p.s.consume()
		return nil, nil
	case psd1TokIdent:
		// PowerShell allows barewords in data files. Treat as string.
		return p.s.consume().text, nil
	case psd1TokEOF:
		return nil, fmt.Errorf("psd1: unexpected EOF")
	default:
		return nil, fmt.Errorf("psd1: unexpected token %q", tok.text)
	}
}

func (p *psd1Parser) parseHashtable() (map[string]interface{}, error) {
	if _, err := p.expect(psd1TokLBrace); err != nil {
		return nil, err
	}
	out := make(map[string]interface{})
	for {
		tok := p.s.peek()
		switch tok.kind {
		case psd1TokRBrace:
			_ = p.s.consume()
			return out, nil
		case psd1TokSemi, psd1TokComma:
			_ = p.s.consume()
			continue
		case psd1TokEOF:
			return nil, fmt.Errorf("psd1: unexpected EOF in hashtable")
		}

		// Key
		keyTok := p.s.consume()
		if keyTok.kind != psd1TokIdent && keyTok.kind != psd1TokString {
			return nil, fmt.Errorf("psd1: expected key, got %q", keyTok.text)
		}
		key := strings.TrimSpace(keyTok.text)
		if _, err := p.expect(psd1TokEq); err != nil {
			return nil, err
		}
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		out[key] = val
	}
}

func (p *psd1Parser) parseArray() ([]interface{}, error) {
	if _, err := p.expect(psd1TokLParen); err != nil {
		return nil, err
	}
	items := make([]interface{}, 0, 4)
	for {
		tok := p.s.peek()
		switch tok.kind {
		case psd1TokRParen:
			_ = p.s.consume()
			return items, nil
		case psd1TokComma, psd1TokSemi:
			_ = p.s.consume()
			continue
		case psd1TokEOF:
			return nil, fmt.Errorf("psd1: unexpected EOF in array")
		}
		v, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		items = append(items, v)
	}
}
