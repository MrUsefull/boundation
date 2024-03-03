package config

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/ilyakaznacheev/cleanenv"
)

var (
	ErrInvalidBaseURL = errors.New("invalid base url - must start with http:// or https://")
	ErrInvalidCreds   = errors.New("invalid creds - must be in format \"apiKey:apiSecret\"")
)

type Config struct {
	Opnsense `yaml:"opnsense"`
	Listen   `yaml:"listen"`
	// Filter is the domains to match for this provider
	DomainFilter `yaml:"filter"`
	LogLevel     slog.Level `yaml:"loglevel" env:"LOG_LEVEL"`
}

type Opnsense struct {
	// BaseURL of OPNSense instance. Must include the protocol
	// eg: https://router.yourdomain.fqdn or http://10.0.0.1
	BaseURL string `yaml:"baseurl" env:"OPNSENSE_BASEURL"`
	// Creds in the form of APIKey:Secret
	// obtained from OPNSense
	Creds string `yaml:"creds" env:"OPNSENSE_CREDS"`
}

type Listen struct {
	// Addr is the address + port we listen on
	Addr string `yaml:"addr" env:"LISTEN_ADDR" env-default:":8080"`
}

type DomainFilter struct {
	// Filter is the domains we want to match and work with
	Filter []string `yaml:"filter" env:"DOMAIN_FILTER"`
	// Exclude is the domains we want to exclude and not touch
	Exclude []string `yaml:"exclude" env:"DOMAIN_EXCLUDE"`
}

func Load(path string) (Config, error) {
	cfg := Config{}
	if err := cleanenv.ReadConfig(path, &cfg); err != nil && !errors.Is(err, io.EOF) {
		return cfg, fmt.Errorf("load config: %w", err)
	}

	return cfg, cfg.Validate()
}

func (cfg *Config) Validate() error {
	if cfg.BaseURL == "" || !hasProtocol(cfg.BaseURL) {
		return fmt.Errorf("%v: %w", cfg.BaseURL, ErrInvalidBaseURL)
	}

	if cfg.Creds == "" || !credFormat(cfg.Creds) {
		return ErrInvalidCreds
	}

	cfg.BaseURL = strings.TrimSuffix(cfg.BaseURL, "\n")
	cfg.Creds = strings.TrimSuffix(cfg.Creds, "\n")

	return nil
}

func hasProtocol(baseURL string) bool {
	https := strings.HasPrefix(baseURL, "https://")
	http := strings.HasPrefix(baseURL, "http://")
	return http || https
}

func credFormat(cred string) bool {
	credSlice := strings.Split(cred, ":")
	return len(credSlice) == 2
}
