package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/cmd/cli/config"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/apserde"
	"github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/rmorlok/authproxy/internal/schema/common"
	"github.com/rmorlok/authproxy/internal/schema/resources/meta"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
	"github.com/rmorlok/authproxy/internal/schema/resources/tools"
	"github.com/rmorlok/authproxy/internal/schema/resources/toolsets"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// cmdGet groups reads of individual Tool and ToolSet management resources.
func cmdGet() *cobra.Command {
	cmd := &cobra.Command{Use: "get", Short: "Get a resource"}
	cmd.AddCommand(
		cmdGetToolResource(tools.ToolKind, "tools", apid.PrefixTool),
		cmdGetToolResource(toolsets.ToolSetKind, "toolsets", apid.PrefixToolSet),
	)
	return cmd
}

// cmdGetToolResource resolves a typed ID or an exact namespaced name, optionally
// selecting one ToolSet generation after resolving its logical identity.
func cmdGetToolResource(kind meta.Kind, collection string, prefix apid.Prefix) *cobra.Command {
	var ns string
	var generation uint64
	cmd := &cobra.Command{
		Use:          collection + " ID_OR_NAME",
		Short:        "Get one " + string(kind),
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
	}
	read := toolResourceReadFlags(cmd)
	cmd.Flags().StringVarP(
		&ns,
		"namespace", // name
		"n",         // shorthand
		"",          // value
		"Exact namespace (required when looking up a name)", // usage
	)
	if kind == toolsets.ToolSetKind {
		cmd.Flags().
			Uint64Var(
				&generation,
				"generation",                         // name
				0,                                    // value
				"Read a specific ToolSet generation", // usage
			)
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("generation") && generation == 0 {
			return fmt.Errorf("--generation must be positive")
		}

		m := meta.ObjectMeta{
			Namespace:  ns,
			Generation: generation,
		}

		if id, err := apid.Parse(args[0]); err == nil && !id.IsNil() {
			if id.ValidatePrefix(prefix) != nil {
				return fmt.Errorf("expected a %s ID", kind)
			}
			m.ID = id.String()
		} else {
			if ns == "" {
				return fmt.Errorf("--namespace is required when looking up a %s name", kind)
			}

			m.Name = common.ResourceName(args[0])
			if err := m.Name.Validate(); err != nil {
				return fmt.Errorf("invalid %s name: %w", kind, err)
			}
		}

		if ns != "" {
			if err := namespace.ValidatePath(ns); err != nil {
				return fmt.Errorf("--namespace must be an exact namespace: %w", err)
			}
		}

		client, printer, err := read.prepare()
		if err != nil {
			return err
		}
		live, err := client.Resolve(cmd.Context(), apply.Document{Kind: kind, Metadata: m})
		if errors.Is(err, apply.ErrResourceNotFound) || (err == nil && live == nil) {
			return fmt.Errorf("%s not found", kind)
		}
		if err != nil {
			return err
		}
		return read.write(cmd, printer, []*apply.LiveResource{live}, true)
	}
	return cmd
}

// cmdListToolResources lists resources across all cursor pages using the
// management endpoint's filters and ordering.
func cmdListToolResources(kind meta.Kind, collection string) *cobra.Command {
	var options apply.ListOptions
	cmd := &cobra.Command{
		Use:          collection,
		Short:        "List " + collection,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
	}
	read := toolResourceReadFlags(cmd)
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", "", "Filter by namespace matcher")
	cmd.Flags().StringVar(&options.Name, "name", "", "Filter by exact name")
	cmd.Flags().StringVarP(&options.LabelSelector, "selector", "l", "", "Filter by label selector")
	cmd.Flags().StringVar(&options.OrderBy, "order", "", "Order by field and optional ASC or DESC")
	if kind == tools.ToolKind {
		cmd.Flags().StringVar(&options.ConnectionID, "connection", "", "Filter by bound connection ID")
	}
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		client, printer, err := read.prepare()
		if err != nil {
			return err
		}
		list, err := client.ListResources(cmd.Context(), kind, options)
		if err != nil {
			return err
		}
		return read.write(cmd, printer, list, false)
	}
	return cmd
}

// toolResourceReadOptions shares signing, timeout, and output conventions
// across the Tool and ToolSet inspection commands.
type toolResourceReadOptions struct {
	resolver     *config.Resolver
	output       string
	expression   string
	allowMissing bool
	timeout      time.Duration
}

// toolResourceReadFlags registers the shared options on a read command.
func toolResourceReadFlags(cmd *cobra.Command) *toolResourceReadOptions {
	read := &toolResourceReadOptions{resolver: config.WithConfigParams(cmd)}
	cmd.Flags().
		StringVarP(
			&read.output,
			"output", // name
			"o",      // shorthand
			"json",   // value
			"Output: name, json, yaml, go-template[-file], jsonpath[-file], jsonpath-as-json",
		)
	cmd.Flags().
		StringVar(
			&read.expression,
			"template", // name
			"",         // value
			"Go-template or JSONPath expression, or path with a file output format",
		)
	cmd.Flags().
		BoolVar(
			&read.allowMissing,
			"allow-missing-template-keys", // name
			true,                          // value
			"Ignore missing fields in templates and JSONPath",
		)
	cmd.Flags().
		DurationVar(
			&read.timeout,
			"request-timeout",           // name
			apply.DefaultRequestTimeout, // value
			"Timeout per cluster request; 0 disables the deadline",
		)

	return read
}

// prepare validates the output expression before resolving credentials and
// constructing the authenticated management client.
func (r *toolResourceReadOptions) prepare() (*apply.Client, applyPrinter, error) {
	if r.timeout < 0 {
		return nil, nil, fmt.Errorf("--request-timeout cannot be negative")
	}

	printer, err := newApplyPrinter(r.output, r.expression, r.allowMissing)
	if err != nil {
		return nil, nil, err
	}

	client, err := r.resolver.ResolveApplyClient(r.timeout)
	return client, printer, err
}

// write formats fully read and sanitized resources. Failed pagination cannot
// leave a partial result on stdout.
func (r *toolResourceReadOptions) write(
	cmd *cobra.Command,
	printer applyPrinter,
	resources []*apply.LiveResource,
	single bool,
) error {
	w := cmd.OutOrStdout()
	if r.output == "name" {
		for _, resource := range resources {
			if _, err := fmt.Fprintf(w, "%s/%s\n", strings.ToLower(string(resource.Kind)), resource.Metadata.ID); err != nil {
				return err
			}
		}
		return nil
	}

	objects := make([]any, 0, len(resources))
	sanitize := apserde.SanitizeJSONForAPI
	if r.output == "yaml" {
		// Use the YAML codec so revisions, generations, and numeric schema
		// values remain numbers instead of quoted JSON number strings.
		sanitize = apserde.SanitizeYAMLForAPI
	}

	for _, resource := range resources {
		object, _, err := sanitize(cmd.Context(), resource.Resource)
		if err != nil {
			return fmt.Errorf("cannot sanitize resource output: %w", err)
		}
		objects = append(objects, object)
	}

	var value any = objects
	if single {
		value = objects[0]
	}

	if printer != nil {
		return printer(w, value)
	}

	if r.output == "yaml" {
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		for _, object := range objects {
			if err := enc.Encode(object); err != nil {
				return err
			}
		}
		return enc.Close()
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	return enc.Encode(value)
}
