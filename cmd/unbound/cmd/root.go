/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/spf13/cobra"
)

var (
	// cfgFile is the path to the config file.
	cfgFile string

	pkgConfig config.Config

	logger = slog.New(
		slog.NewTextHandler(
			os.Stderr,
			&slog.HandlerOptions{
				AddSource: true,
				Level:     pkgConfig.LogLevel,
				ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
					if a.Key == slog.SourceKey {
						s, ok := a.Value.Any().(*slog.Source)
						if !ok {
							return a
						}
						s.File = path.Base(s.File)
					}
					return a
				},
			},
		))
)

const (
	hostsFlag         = "host"
	targetsFlag       = "target"
	defaultConfigFile = "/unbound.yml"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "unbound",
	Short: "Manage OPNSense unbound DNS",
	Long:  `unbound exposes a CLI for CRUD operations on OPNsense Unbound DNS overrides`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

//nolint:gochecknoinits // cobra is outside of our control
func init() {
	homedir, err := os.UserHomeDir()
	if err != nil {
		rootCmd.PersistentFlags().StringVar(&cfgFile, "config", defaultConfigFile, "config file path")
	} else {
		cfgFile = path.Join(homedir, ".unbound", defaultConfigFile)
		rootCmd.PersistentFlags().StringVar(&cfgFile, "config", cfgFile, "config file path")
	}

	setCreateCmdFlags(upsertCMD)
	setDeleteCmdFlags(deleteCMD)
	rootCmd.AddCommand(upsertCMD)
	rootCmd.AddCommand(configureCMD)
	rootCmd.AddCommand(readCMD)
	rootCmd.AddCommand(deleteCMD)
}

type runEFn func(cmd *cobra.Command, args []string) error

// configured decorates wrapped to ensure configuration is loaded correctly.
func configured(wrapped runEFn) runEFn {
	return func(cmd *cobra.Command, args []string) error {
		loadedCfg, err := config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("configuration: %w", err)
		}
		pkgConfig = loadedCfg
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{AddSource: true, Level: pkgConfig.LogLevel}))
		return wrapped(cmd, args)
	}
}
