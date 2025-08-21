package api

import (
	"firestige.xyz/otus/internal/otus/module/capture/codec"
	"firestige.xyz/otus/internal/plugin"
)

type Parser interface {
	codec.Parser
	plugin.Plugin
}
