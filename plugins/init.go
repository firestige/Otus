package plugin

import (
	"firestige.xyz/otus/plugin/parser"
	"firestige.xyz/otus/plugin/reporter"
)

func SeekAndRegisterModules() {
	parser.RegisterExtendedParserModule()
	reporter.RegisterExtendedReporterModule()
}
