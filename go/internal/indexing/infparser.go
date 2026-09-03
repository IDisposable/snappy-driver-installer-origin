package indexing

import (
	"bytes"
	"strings"

	"sdio/internal/common"
)

// InfParser tokenizes a byte range of .inf file text that has already
// been isolated to one section (e.g. everything between a
// "[Manufacturer]" line and the next "[...]" line), extracting
// "key = field,field,field" style lines with %string% substitution.
// Ported from Parser in indexing.cpp.
//
// Unlike the original, which keeps strBeg/strEnd as pointers that get
// redirected into different buffers by substitution (the source
// buffer, a separate string table, or a scratch buffer for
// multi-substitution tokens), ParseItem/ParseField here return the
// final, already-substituted token as a plain Go string: Go strings
// are immutable values, so there's no need to track "which buffer does
// this token currently point into."
//
// stringList holds %name%->value substitutions (from a driver pack's
// [Strings] section), with lowercased keys, matching the original's
// case-insensitive lookup.
type InfParser struct {
	data               []byte
	stringList         map[string]string
	blockBeg, blockEnd int
}

// NewInfParser creates a parser over data[blockBeg:blockEnd].
func NewInfParser(data []byte, blockBeg, blockEnd int, stringList map[string]string) *InfParser {
	return &InfParser{data: data, blockBeg: blockBeg, blockEnd: blockEnd, stringList: stringList}
}

// parseWhitespace advances past spaces, tabs, comments (";" to end of
// line), and backslash-CRLF line continuations. If eatNewline is
// false, it stops at (without consuming) a line ending. Ported from
// Parser::parseWhitespace.
func (p *InfParser) parseWhitespace(eatNewline bool) {
	for p.blockBeg < p.blockEnd {
		switch p.data[p.blockBeg] {
		case '\n', '\r':
			if !eatNewline {
				return
			}
			p.blockBeg++
		case ' ', '\t':
			p.blockBeg++
		case ';':
			p.blockBeg++
			for p.blockBeg < p.blockEnd && p.data[p.blockBeg] != '\n' && p.data[p.blockBeg] != '\r' {
				p.blockBeg++
			}
		case '\\':
			if p.blockBeg+3 < p.blockEnd && p.data[p.blockBeg+1] == '\r' && p.data[p.blockBeg+2] == '\n' {
				p.blockBeg += 3
				continue
			}
			return
		default:
			return
		}
	}
}

// finalizeToken trims trailing spaces/tabs, strips one leading and one
// trailing '"' if present (independently, not as a matched pair), and
// applies %string% substitution - ported from the trimtoken()+subStr()
// sequence every token goes through in the original.
func (p *InfParser) finalizeToken(beg, end int) (string, bool) {
	for end > beg && (p.data[end-1] == ' ' || p.data[end-1] == '\t') {
		end--
	}
	if end > beg && p.data[beg] == '"' {
		beg++
	}
	if end > beg && p.data[end-1] == '"' {
		end--
	}
	if beg > end {
		return "", false
	}
	return p.substitute(p.data[beg:end]), true
}

// substitute performs %name% replacement on tok, ported from
// Parser::subStr. A token that is exactly "%name%" (or "%name" missing
// its closing '%') is looked up directly; otherwise, every "%name%"
// found anywhere in tok is replaced piecewise, and unmatched '%'
// characters are left as-is. A token with no successful substitution
// anywhere is returned unchanged, matching the original leaving
// strBeg/strEnd untouched when its "flag" stays 0.
func (p *InfParser) substitute(tok []byte) string {
	if len(p.stringList) == 0 || len(tok) == 0 {
		return string(tok)
	}

	if tok[0] == '%' {
		inner := tok[1:]
		if len(inner) > 0 && inner[len(inner)-1] == '%' {
			inner = inner[:len(inner)-1]
		}
		if v, ok := p.stringList[strings.ToLower(string(inner))]; ok {
			return v
		}
	}

	if !bytes.ContainsRune(tok, '%') {
		return string(tok)
	}

	var out strings.Builder
	replaced := false
	i := 0
	for i < len(tok) {
		start := i
		for i < len(tok) && tok[i] != '%' {
			i++
		}
		out.Write(tok[start:i])
		if i >= len(tok) {
			break
		}
		j := i + 1
		for j < len(tok) && tok[j] != '%' {
			j++
		}
		if j < len(tok) {
			name := strings.ToLower(string(tok[i+1 : j]))
			if v, ok := p.stringList[name]; ok {
				out.WriteString(v)
				replaced = true
				i = j + 1
				continue
			}
		}
		out.WriteByte(tok[i])
		i++
	}
	if !replaced {
		return string(tok)
	}
	return out.String()
}

// ParseItem finds the "key" of the next "key = ..." line in the
// remaining block, silently skipping blank or malformed lines - ported
// from Parser::parseItem. ok is false once the block is exhausted.
func (p *InfParser) ParseItem() (key string, ok bool) {
	p.parseWhitespace(true)
	strBeg := p.blockBeg
	i := p.blockBeg

	for i < p.blockEnd-1 {
		switch p.data[i] {
		case '=':
			p.blockBeg = i
			return p.finalizeToken(strBeg, i)
		case '\n', '\r':
			p.blockBeg = i
			i++
			p.parseWhitespace(true)
			i = p.blockBeg
			strBeg = p.blockBeg
		default:
			i++
		}
	}
	return "", false
}

// ParseField parses the next comma-delimited field after a "=" (from
// ParseItem) or a previous field, supporting a quoted "str" form.
// ported from Parser::parseField. ok is false at the end of the field
// list (a line ending or comment with no more commas).
func (p *InfParser) ParseField() (value string, ok bool) {
	if p.blockBeg >= p.blockEnd {
		return "", false
	}
	c := p.data[p.blockBeg]
	if c != '=' && c != ',' {
		return "", false
	}
	p.blockBeg++
	p.parseWhitespace(false)

	i := p.blockBeg
	if i < p.blockEnd && p.data[i] == '"' {
		strBeg := i + 1
		i++
		for i < p.blockEnd {
			switch p.data[i] {
			case '\r', '\n':
				i++
			case '"':
				strEnd := i
				p.blockBeg = strEnd + 1
				if strBeg > strEnd {
					return "", false
				}
				return p.substitute(p.data[strBeg:strEnd]), true
			default:
				i++
			}
		}
		return "", false
	}

	strBeg := i
	for i < p.blockEnd {
		switch p.data[i] {
		case '\n', '\r', ';', ',':
			delim := p.data[i]
			p.blockBeg = i
			tok, valid := p.finalizeToken(strBeg, i)
			if !valid {
				return "", false
			}
			return tok, len(tok) != 0 || delim == ','
		default:
			i++
		}
	}
	return "", false
}

// ParseHexByte decomposes a field value like "0x1A" into a single byte
// value (0-255), ported from Parser::readHex. It skips a "0x"-style
// prefix, then reads at most two hex digits.
func ParseHexByte(s string) (val int, rest string) {
	i := 0
	for i < len(s) && (s[i] == '0' || s[i] == 'x') {
		i++
	}
	hexDigit := func(c byte) int {
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c <= '9' {
			return int(c - '0')
		}
		return int(c-'A') + 10
	}
	if i < len(s) {
		val = hexDigit(s[i])
	}
	i++
	if i < len(s) {
		val = val<<4 + hexDigit(s[i])
	}
	if i > len(s) {
		i = len(s)
	}
	return val, s[i:]
}

// ParseNumber consumes a leading run of digits from s, plus one
// trailing delimiter character if present, ported from
// Parser::readNumber. It decomposes a single already-extracted field
// value like "01/02/2024" or "1.2.3.4" one part at a time - callers
// chain calls, threading rest through (see ParseDate/ParseVersion).
func ParseNumber(s string) (n int, rest string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	if i < len(s) {
		i++
	}
	return n, s[i:]
}

// ParseDate decomposes a field value into a month/day/year date,
// ported from Parser::readDate. The .inf DriverVer convention is
// mm/dd/yyyy, hence the month-day-year read order despite
// common.Version.SetDate taking (day, month, year).
func ParseDate(s string) common.Version {
	i := 0
	for i < len(s) && !(s[i] >= '0' && s[i] <= '9') {
		i++
	}
	s = s[i:]

	var month, day, year int
	month, s = ParseNumber(s)
	day, s = ParseNumber(s)
	year, _ = ParseNumber(s)

	var v common.Version
	v.SetDate(day, month, year)
	return v
}

// ParseVersionNumber decomposes a field value into a four-part version
// number, ported from Parser::readVersion.
func ParseVersionNumber(s string) common.Version {
	var v1, v2, v3, v4 int
	v1, s = ParseNumber(s)
	v2, s = ParseNumber(s)
	v3, s = ParseNumber(s)
	v4, _ = ParseNumber(s)

	var v common.Version
	v.SetVersion(v1, v2, v3, v4)
	return v
}
