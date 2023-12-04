package config

import (
	"log"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig will load config attributes from a yaml file
func LoadConfig(cfn string) (*ServiceConfig, error) {

	confContent, err := os.ReadFile(cfn)
	if err != nil {
		log.Panicf("Failed to read configuration file: %v", err)
		return nil, err
	}

	confContent = []byte(os.ExpandEnv(string(confContent)))
	sc := ServiceConfig{}

	if err := yaml.Unmarshal(confContent, &sc); err != nil {
		log.Printf("Failed to unmarshal configuration file: %v", err)
		return nil, err
	}

	return &sc, nil
}
