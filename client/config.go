package client

import (
	"github.com/go-msvc/ms"
)

type Config struct {
}

func (c *Config) Validate() error {
	return nil
}

func (c Config) Create() ms.Client {
	return client{}
}
