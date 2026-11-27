package main

import (
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/spf13/cobra"
)

func cmdList() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entities",
	}

	cmd.AddCommand(cmdListConnections())
	cmd.AddCommand(cmdListConnectors())
	cmd.AddCommand(cmdListToolResources(tools.ToolKind, "tools"))
	cmd.AddCommand(cmdListToolResources(toolsets.ToolSetKind, "toolsets"))

	return cmd
}
