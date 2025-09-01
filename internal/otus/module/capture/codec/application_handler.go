package codec

import (
	"fmt"
)

type parserComposite struct {
	parsers []Parser
}

// NewParserComposite 创建解析器组合，自动发现并注册所有可用的 parser 插件
func NewParserComposite(parsers ...Parser) Parser {
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

func (p *parserComposite) Extract(content []byte) (msg []byte, consumed int, ptype string, err error) {
	for _, parser := range p.parsers {
		if parser.Detect(content) {
			return parser.Extract(content)
		}
	}
	return nil, 0, "", fmt.Errorf("no parser found for content")
}

func (p *parserComposite) Reset() {
	for _, parser := range p.parsers {
		parser.Reset()
	}
}
