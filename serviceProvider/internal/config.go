package internal

import (
	"github.com/BurntSushi/toml"
)

type SPConf struct {
	API      API
	Metrics  Metrics
	Loglevel string
}

type API struct {
	Host   string
	Port   uint16
	Prefix string
}

type Metrics struct {
	Host string
	Port uint16
	Path string
}

func ReadConfig(path string) (SPConf, error) {
	var conf SPConf

	_, err := toml.DecodeFile(path, &conf)
	if err != nil {
		return conf, err
	}

	return conf, nil
}
