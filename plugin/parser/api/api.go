package api

import (
	"firestige.xyz/otus/internal/otus/capture/codec"
	"firestige.xyz/otus/internal/plugin"
)

type Parser interface {
	codec.Parser
	plugin.Plugin
}
