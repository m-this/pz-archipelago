package steam

import (
	"fmt"
	"strings"
)

// Node is one key of a Valve KeyValues (VDF) text file: a string value or
// a block of children. Keys compare case-insensitively, as Steam reads them.
type Node struct {
	Value    string
	Children map[string]*Node
}

// Child returns a nested node by path, or nil.
func (n *Node) Child(path ...string) *Node {
	for _, key := range path {
		if n == nil || n.Children == nil {
			return nil
		}
		n = n.Children[strings.ToLower(key)]
	}
	return n
}

// ParseVDF reads a KeyValues text file. It has no limit beyond its input:
// every step consumes at least one byte.
func ParseVDF(text string) (*Node, error) {
	p := &vdfParser{text: text}
	root := &Node{Children: map[string]*Node{}}
	if err := p.block(root, false); err != nil {
		return nil, fmt.Errorf("vdf at byte %d: %w", p.pos, err)
	}
	return root, nil
}

type vdfParser struct {
	text string
	pos  int
}

func (p *vdfParser) block(into *Node, nested bool) error {
	for {
		tok, quoted, err := p.token()
		if err != nil {
			return err
		}
		switch {
		case tok == "" && !quoted:
			if nested {
				return fmt.Errorf("unclosed block")
			}
			return nil
		case tok == "}" && !quoted:
			if !nested {
				return fmt.Errorf("unexpected }")
			}
			return nil
		}
		value, valueQuoted, err := p.token()
		if err != nil {
			return err
		}
		child := &Node{}
		switch {
		case value == "{" && !valueQuoted:
			child.Children = map[string]*Node{}
			if err := p.block(child, true); err != nil {
				return err
			}
		case value == "" && !valueQuoted, value == "}" && !valueQuoted:
			return fmt.Errorf("key %q has no value", tok)
		default:
			child.Value = value
		}
		into.Children[strings.ToLower(tok)] = child
	}
}

// token returns the next token; "" unquoted means the end of the input.
func (p *vdfParser) token() (string, bool, error) {
	p.skipSpaceAndComments()
	if p.pos >= len(p.text) {
		return "", false, nil
	}
	switch c := p.text[p.pos]; c {
	case '{', '}':
		p.pos++
		return string(c), false, nil
	case '"':
		return p.quoted()
	}
	start := p.pos
	for p.pos < len(p.text) && !strings.ContainsRune(" \t\r\n{}\"", rune(p.text[p.pos])) {
		p.pos++
	}
	return p.text[start:p.pos], false, nil
}

func (p *vdfParser) quoted() (string, bool, error) {
	p.pos++
	var b strings.Builder
	for p.pos < len(p.text) {
		c := p.text[p.pos]
		p.pos++
		switch c {
		case '"':
			return b.String(), true, nil
		case '\\':
			if p.pos >= len(p.text) {
				return "", false, fmt.Errorf("escape at end of input")
			}
			e := p.text[p.pos]
			p.pos++
			switch e {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(e)
			}
		default:
			b.WriteByte(c)
		}
	}
	return "", false, fmt.Errorf("unterminated string")
}

func (p *vdfParser) skipSpaceAndComments() {
	for p.pos < len(p.text) {
		switch {
		case strings.ContainsRune(" \t\r\n", rune(p.text[p.pos])):
			p.pos++
		case strings.HasPrefix(p.text[p.pos:], "//"):
			end := strings.IndexByte(p.text[p.pos:], '\n')
			if end < 0 {
				p.pos = len(p.text)
				return
			}
			p.pos += end + 1
		default:
			return
		}
	}
}
