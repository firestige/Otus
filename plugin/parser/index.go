package parser

import (
	"reflect"

	"firestige.xyz/otus/internal/plugin"
	"firestige.xyz/otus/plugin/parser/api"
	"firestige.xyz/otus/plugin/parser/sip"
)

func RegisterExtendedParserModule() {
	// 注册 parser 插件类型
	plugin.RegisterPluginType(reflect.TypeOf((*api.Parser)(nil)).Elem())

	// 注册具体的 parser 实现
	parsers := []api.Parser{
		&sip.SipParser{},
	}

	for _, p := range parsers {
		plugin.RegisterPlugin(p)
	}
}
