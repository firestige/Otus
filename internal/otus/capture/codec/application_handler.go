package codec

import (
	"fmt"
	"reflect"

	"firestige.xyz/otus/internal/plugin"
)

type parserComposite struct {
	parsers []Parser
}

// NewParserComposite 创建解析器组合，自动发现并注册所有可用的 parser 插件
func NewParserComposite(parsers ...Parser) Parser {
	composite := &parserComposite{}

	// 如果传入了特定的 parser，则使用传入的
	if len(parsers) > 0 {
		composite.parsers = parsers
		return composite
	}

	// 否则自动发现所有已注册的 parser 插件
	parserType := reflect.TypeOf((*Parser)(nil)).Elem()
	pluginValues := plugin.GetPluginsByType(parserType)

	for _, pluginValue := range pluginValues {
		if pluginValue.IsValid() {
			if p, ok := pluginValue.Interface().(Parser); ok {
				composite.parsers = append(composite.parsers, p)
			}
		}
	}

	return composite
}

// NewParserCompositeByNames 根据指定的 parser 名称创建组合
func NewParserCompositeByNames(names ...string) Parser {
	composite := &parserComposite{}
	parserType := reflect.TypeOf((*Parser)(nil)).Elem()

	for _, name := range names {
		pluginValue := plugin.GetPluginByName(parserType, name)
		if pluginValue.IsValid() {
			if p, ok := pluginValue.Interface().(Parser); ok {
				composite.parsers = append(composite.parsers, p)
			}
		}
	}

	return composite
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
	for _, parser := range p.parsers {
		parser.Reset()
	}
}
