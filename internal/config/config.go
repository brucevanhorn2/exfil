package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Host struct {
	Name         string `yaml:"name"`
	Hostname     string `yaml:"hostname"`
	Port         int    `yaml:"port"`
	User         string `yaml:"user"`
	IdentityFile string `yaml:"identity_file,omitempty"`
	RemotePath   string `yaml:"remote_path,omitempty"`
}

type Config struct {
	Hosts []Host `yaml:"hosts"`
}

func DefaultPort() int { return 22 }

func Path() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "exfil", "hosts.yaml"), nil
}

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return &Config{}, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}

	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	return os.WriteFile(p, data, 0600)
}
