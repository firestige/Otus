package consolelog

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/internal/log"
	otus "firestige.xyz/otus/internal/otus/api"
)

var (
	Name     = "consolelog"
	ShowName = "Console log"
)

type Console struct {
	config.CommonFields
}

func (c *Console) Name() string {
	return Name
}

func (c *Console) DefaultConfig() string {
	return ``
}

func (c *Console) PostConstruct() error {
	return nil
}

// 尝试简单将报文内容输出到日志，可以用于检查前面的流程是否工作正常
func (c *Console) Report(batch otus.BatchPacket) error {
	for _, p := range batch {
		log.GetLogger().Infof("reporting packet: %s", string(p.Payload))
	}
	return nil
}

// 这个地方的 Protocol 后面应该被常量替代
func (c *Console) SupportProtocol() string {
	return "SIP"
}

func (c *Console) ReportType() {
}
