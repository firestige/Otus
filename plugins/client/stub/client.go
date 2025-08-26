package stub

import (
	"firestige.xyz/otus/internal/config"
	"firestige.xyz/otus/plugins/client/api"
)

const (
	Name     = "stub-client"
	ShowName = "stub client"
)

type StubClient struct {
	config.CommonFields
}

func (c *StubClient) Name() string {
	return Name
}

func (c *StubClient) ShowName() string {
	return ShowName
}

func (c *StubClient) DefaultConfig() string {
	return ""
}

func (c *StubClient) GetConnectedClient() interface{} {
	return nil
}

func (c *StubClient) RegisterListener(chan<- api.ClientStatus) {
}

func (c *StubClient) PostConstruct() error {
	return nil
}

func (c *StubClient) Start() error {
	return nil
}

func (c *StubClient) Close() error {
	return nil
}
