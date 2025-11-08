package main

import (
	"github.com/spf13/cobra"
)

func cmdList() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entities",
	}

	cmd.AddCommand(cmdListConnections())
	cmd.AddCommand(cmdListConnectors())

	return cmd
}
