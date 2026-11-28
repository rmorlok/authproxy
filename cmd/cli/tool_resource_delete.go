package main

import (
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/spf13/cobra"
)

// cmdDelete groups operations for deleting a single management resource.
func cmdDelete() *cobra.Command {
	cmd := &cobra.Command{Use: "delete", Short: "Delete a resource"}
	cmd.AddCommand(
		cmdDeleteToolResource(tools.ToolKind, "tools", apid.PrefixTool, "Delete one standalone Tool"),
		cmdDeleteToolResource(toolsets.ToolSetKind, "toolsets", apid.PrefixToolSet, "Delete a ToolSet with all of its generations"),
	)
	return cmd
}

// cmdDeleteToolResource resolves the target once and deletes its immutable ID.
// Only an acknowledged deletion produces a success result on stdout.
func cmdDeleteToolResource(kind meta.Kind, collection string, prefix apid.Prefix, short string) *cobra.Command {
	var ns string
	cmd := &cobra.Command{
		Use:          collection + " ID_OR_NAME",
		Short:        short,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
	}
	options := toolResourceFlags(cmd, "")
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "Exact namespace (required when looking up a name)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		target, err := toolResourceTarget(kind, prefix, args[0], ns)
		if err != nil {
			return err
		}
		client, printer, err := options.prepare()
		if err != nil {
			return err
		}
		id, err := resolveToolResourceID(cmd.Context(), client, target)
		if err != nil {
			return err
		}
		if err := client.DeleteToolResource(cmd.Context(), kind, id); err != nil {
			return err
		}

		result := apply.Result{
			Source:    "delete",
			Kind:      kind,
			Identity:  id.String(),
			Operation: "delete",
			Status:    "deleted",
		}
		results := []apply.Result{result}
		if printer != nil {
			err = printer(cmd.OutOrStdout(), results)
		} else {
			err = writeApplyResults(cmd, options.output, results)
		}
		if err != nil {
			return fmt.Errorf("%s was deleted but output failed: %w", apply.ResultName(result), err)
		}
		return nil
	}
	return cmd
}
