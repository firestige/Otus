package codec

import (
	"fmt"

	parser "firestige.xyz/otus/plugin/parser/api"
)

type parserComposite struct {
	parsers []parser.Parser
}

func NewParserComposite(parsers ...parser.Parser) parser.Parser {
	return &parserComposite{
		parsers: parsers,
	}
}

func (p *parserComposite) Detect(content []byte) bool {
	for _, parser := range p.parsers {
		if parser.Detect(content) {
			return true
		}
	}
	return false
}

func (p *parserComposite) Extract(content []byte) (msg []byte, consumed int, err error) {
	for _, parser := range p.parsers {
		if parser.Detect(content) {
			return parser.Extract(content)
		}
	}
	return nil, 0, fmt.Errorf("no parser found for content")
}

func (p *parserComposite) Reset() {

}
