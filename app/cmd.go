package app

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// NewCommand returns the root cobra command.
func NewCommand() *cobra.Command {
	var config string

	cmd := &cobra.Command{
		Use:               Name,
		Short:             "Read-only dbt catalog gate (HTTP + MCP)",
		Version:           "v0.1.0",
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := Run(config); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return err
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&config, "config", "c", "config.yaml", "Config file path")
	return cmd
}
