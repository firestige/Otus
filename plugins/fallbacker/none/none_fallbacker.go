package none

import (
	"firestige.xyz/otus/internal/config"
	otus "firestige.xyz/otus/internal/otus/api"
	reporter "firestige.xyz/otus/plugins/reporter/api"
)

const (
	Name     = "none-fallbacker"
	ShowName = "Nonw Fallbacker"
)

type Fallbacker struct {
	config.CommonFields
}

func (f *Fallbacker) Name() string {
	return Name
}

func (f *Fallbacker) ShowName() string {
	return ShowName
}

func (f *Fallbacker) DefaultConfig() string {
	return ``
}

func (f *Fallbacker) Fallback(data *otus.BatchePacket, reporter reporter.ReporterFunc) bool {
	return true
}
