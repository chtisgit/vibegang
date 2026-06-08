package config

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type AgentConfig struct {
	Name         string   `yaml:"name"`
	Email        string   `yaml:"email"`
	Role         string   `yaml:"role"`
	SystemPrompt string   `yaml:"system_prompt"`
	Tools        []string `yaml:"tools"`
	Model        string   `yaml:"model,omitempty"`
}

type Config struct {
	CompanyName string        `yaml:"company_name"`
	MailDomain  string        `yaml:"mail_domain"`
	SSHKeyPath  string        `yaml:"ssh_key_path"`
	UserName    string        `yaml:"user_name"`
	UserEmail   string        `yaml:"user_email"`
	Model       string        `yaml:"model"`
	Agents      []AgentConfig `yaml:"agents"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) GetCompanyShortform() string {
	var sb strings.Builder
	for _, r := range c.CompanyName {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	shortform := sb.String()
	if len(shortform) > 12 {
		shortform = shortform[:12]
	}
	return shortform
}

func (c *Config) GetPostgresContainerName() string {
	return "vibegang-" + c.GetCompanyShortform() + "-postgres"
}
