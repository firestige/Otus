package plugin

import (
	"firestige.xyz/otus/plugins/client"
	"firestige.xyz/otus/plugins/fallbacker"
	"firestige.xyz/otus/plugins/filter"
	"firestige.xyz/otus/plugins/parser"
	"firestige.xyz/otus/plugins/reporter"
)

func SeekAndRegisterModules() {
	client.RegisterExtendedClientModule()
	filter.RegisterExtendedFilterModule()
	fallbacker.RegisterExtendedFallbackerModule()
	parser.RegisterExtendedParserModule()
	reporter.RegisterExtendedReporterModule()
}
