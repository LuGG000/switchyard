package main

import (
	"github.com/spf13/cobra"

	"github.com/LuGG000/switchyard/internal/config"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the default config file",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.Path()
			if err != nil {
				return err
			}
			written, err := config.WriteDefault(path)
			if err != nil {
				return err
			}
			if written {
				printf(cmd, "Created %s\n", path)
			} else {
				printf(cmd, "Config already exists: %s\n", path)
			}
			println(cmd, "Next: switchyard add <name>")
			return nil
		},
	}
}
