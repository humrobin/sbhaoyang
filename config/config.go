package config

import (
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"path/filepath"
)

type ProberMeshConfig struct {
	ProberConfigs       []*ProberConfig `yaml:"prober_configs"`
}

type ProberConfig struct {
	ProberType string   `yaml:"prober_type"`
	Region     string   `yaml:"region"`
	Targets    []string `yaml:"targets"`
}

var cfg *ProberMeshConfig

func InitConfig(path string) error {
	abs, err := os.Getwd()
	if err != nil {
		return err
	}

	cfgPath := filepath.Join(abs, path)
	config, err := loadFile(cfgPath)
	if err != nil {
		return err
	}
	cfg = config
	return nil
}

func Get() *ProberMeshConfig {
	if cfg == nil {
		// 防止不指定配置参数时遍历pcs报错
		return &ProberMeshConfig{ProberConfigs:nil}
	}
	return cfg
}

func loadFile(fileName string) (*ProberMeshConfig, error) {
	bytes, err := ioutil.ReadFile(fileName)
	if err != nil {
		return nil, err
	}
	return load(bytes)
}

func load(bytes []byte) (*ProberMeshConfig, error) {
	cfg := &ProberMeshConfig{}
	err := yaml.Unmarshal(bytes, cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
