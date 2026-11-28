package main

import (
	"context"
	"fmt"

	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/spf13/cobra"
)

// cmdToolSets groups operations that act on a ToolSet or one of its generations.
func cmdToolSets() *cobra.Command {
	cmd := &cobra.Command{Use: "toolsets", Short: "Manage ToolSets"}
	generations := &cobra.Command{Use: "generations", Short: "Manage ToolSet generations"}

	var order string
	list := cmdToolSetGeneration(
		"list", "List a ToolSet's generations", false,
		func(ctx context.Context, client *apply.Client, id apid.ID) ([]*apply.LiveResource, error) {
			return client.ListToolSetGenerations(ctx, id, order)
		},
	)
	list.Flags().StringVar(&order, "order", "", "Order generations by field and optional ASC or DESC")

	create := cmdToolSetGeneration(
		"create", "Create a draft cloned from the newest generation", true,
		func(ctx context.Context, client *apply.Client, id apid.ID) ([]*apply.LiveResource, error) {
			draft, err := client.CreateToolSetGeneration(ctx, id)
			return []*apply.LiveResource{draft}, err
		},
	)

	var generation uint64
	publish := cmdToolSetGeneration(
		"publish", "Publish a specific draft generation", true,
		func(ctx context.Context, client *apply.Client, id apid.ID) ([]*apply.LiveResource, error) {
			published, err := client.PublishToolSetGeneration(ctx, id, generation)
			return []*apply.LiveResource{published}, err
		},
	)
	publish.Flags().Uint64Var(&generation, "generation", 0, "Draft generation to publish (required)")
	publish.PreRunE = func(_ *cobra.Command, _ []string) error {
		if generation == 0 {
			return fmt.Errorf("--generation must be positive")
		}
		return nil
	}

	generations.AddCommand(list, create, cmdUpdateToolSetGeneration(), publish)
	cmd.AddCommand(generations)
	return cmd
}

// cmdUpdateToolSetGeneration loads a complete replacement definition before
// resolving the target, then submits it to the explicitly addressed draft.
func cmdUpdateToolSetGeneration() *cobra.Command {
	var generation uint64
	var filename string
	var definition *toolsets.ToolSetDefinition

	cmd := cmdToolSetGeneration(
		"update", // verb
		"Replace a draft generation's definition", // short
		true, // mutation
		func(ctx context.Context, client *apply.Client, id apid.ID) ([]*apply.LiveResource, error) {
			updated, err := client.UpdateToolSetGeneration(ctx, id, generation, definition)
			return []*apply.LiveResource{updated}, err
		},
	)

	cmd.Flags().Uint64Var(
		&generation,
		"generation",                            // name
		0,                                       // value
		"Draft generation to update (required)", // usage
	)
	cmd.Flags().StringVarP(
		&filename,
		"filename", // name
		"f",        // shorthand
		"",         // value
		"ToolSet definition in YAML or JSON (use - for stdin)", // usage
	)
	cmd.PreRunE = func(cmd *cobra.Command, _ []string) error {
		if generation == 0 {
			return fmt.Errorf("--generation must be positive")
		}

		if filename == "" {
			return fmt.Errorf("--filename is required")
		}

		var err error
		definition, err = apply.LoadToolSetDefinition(filename, cmd.InOrStdin())

		return err
	}

	return cmd
}

// cmdToolSetGeneration resolves a ToolSet once, then runs one generation
// operation on its immutable ID. An ID without a namespace needs no separate
// lookup, so the generation endpoint can enforce its own permissions directly.
func cmdToolSetGeneration(
	verb string,
	short string,
	mutation bool,
	request func(context.Context, *apply.Client, apid.ID) ([]*apply.LiveResource, error),
) *cobra.Command {
	var ns string
	cmd := &cobra.Command{
		Use:          verb + " ID_OR_NAME",
		Short:        short,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
	}
	options := toolResourceFlags(cmd)
	cmd.Flags().StringVarP(&ns, "namespace", "n", "", "Exact namespace (required when looking up a name)")

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		target, err := toolResourceTarget(toolsets.ToolSetKind, apid.PrefixToolSet, args[0], ns)
		if err != nil {
			return err
		}
		client, printer, err := options.prepare()
		if err != nil {
			return err
		}

		id := apid.ID(target.Metadata.ID)
		if id.IsNil() || ns != "" {
			live, err := resolveToolResource(cmd.Context(), client, target)
			if err != nil {
				return err
			}
			id = apid.ID(live.Metadata.ID)
		}

		resources, err := request(cmd.Context(), client, id)
		if err != nil {
			return err
		}
		if err := options.write(cmd, printer, resources, mutation); err != nil {
			if mutation {
				// The response already confirmed the mutation. Output errors
				// must not suggest that the action needs to be replayed.
				return fmt.Errorf("ToolSet generation %d operation succeeded but output failed: %w", resources[0].Metadata.Generation, err)
			}
			return err
		}
		return nil
	}
	return cmd
}
