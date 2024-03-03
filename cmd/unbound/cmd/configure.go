package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var ErrRequired = errors.New("required")

var configureCMD = &cobra.Command{
	Use:     "configure",
	Short:   "interactive config file generator",
	Example: exampleUpsert,
	RunE:    runConfigure,
}

func runConfigure(_ *cobra.Command, _ []string) error {
	return readWriteConfig(os.Stdin)
}

func readWriteConfig(reader io.Reader) error {
	cfgFilePath, err := getCfgPath(reader)
	if err != nil {
		return err
	}
	fmt.Printf("%q will be used to output the generated config\n", cfgFilePath)

	cfg, err := getCfg(reader)
	if err != nil {
		return err
	}

	return writeCfg(cfgFilePath, cfg)
}

func getCfgPath(reader io.Reader) (string, error) {
	cfgFilePath, err := stringInput(reader, fmt.Sprintf("config file path [default: %q]: ", cfgFile))
	if err != nil {
		return "", fmt.Errorf("need output path: %w", err)
	}
	if cfgFilePath == "" {
		return cfgFile, nil
	}
	cfgFilePath, _ = strings.CutSuffix(cfgFilePath, "\n")
	return cfgFilePath, nil
}

func getCfg(reader io.Reader) (config.Config, error) {
	// #1: In the future generate a menu for all options. Required min is enough for now.
	// probably want reflection?
	cfg := config.Config{}
	baseURL, err := readBaseURL(reader)
	if err != nil {
		return cfg, err
	}
	cfg.BaseURL = baseURL

	secret, err := readOPNSenseSecret(reader)
	if err != nil {
		return cfg, err
	}
	cfg.Creds = secret

	if err := cfg.Validate(); err != nil {
		return cfg, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

func writeCfg(cfgFilePath string, cfg config.Config) error {
	yamlBytes, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("failed to convert to yaml: %w", err)
	}

	cfgdir := path.Dir(cfgFilePath)
	if err := os.MkdirAll(cfgdir, 0700); err != nil {
		return fmt.Errorf("create cfg dir: %w", err)
	}

	if err := os.WriteFile(cfgFilePath, yamlBytes, os.FileMode(0600)); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

func readBaseURL(reader io.Reader) (string, error) {
	return requiredStringInput(reader, "OPNSense BaseURL")
}

func readOPNSenseSecret(reader io.Reader) (string, error) {
	return requiredStringInput(reader, "OPNSense credentials")
}

func requiredStringInput(reader io.Reader, label string) (string, error) {
	value, err := stringInput(reader, label)
	if err != nil {
		return value, err
	}
	if value == "" {
		return value, fmt.Errorf("%s: %w", label, ErrRequired)
	}
	return value, nil
}

func stringInput(reader io.Reader, label string) (string, error) {
	fmt.Printf("Please enter %v: ", label)
	buffReader := bufio.NewReader(reader)
	value, err := buffReader.ReadString('\n')
	if isReadError(err) {
		return "", fmt.Errorf("require %s: %w", label, err)
	}
	return strings.TrimSuffix(value, "\n"), nil
}

func isReadError(err error) bool {
	return err != nil && !errors.Is(err, io.EOF)
}
