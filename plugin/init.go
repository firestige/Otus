package plugin

import (
	"firestige.xyz/otus/plugin/parser"
	"firestige.xyz/otus/plugin/sender"
)

func SeekAndRegisterModules() {
	parser.RegisterExtendedParserModule()
	sender.RegisterExtendedSenderModule()
}
