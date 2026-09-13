package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/cmd/cli/config"

	"github.com/rmorlok/authproxy/internal/apply"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func cmdApply() *cobra.Command {
	var options apply.Options
	var dryRun, output, validation string
	var overwrite bool
	var timeout time.Duration
	var resolver *config.Resolver
	cmd := &cobra.Command{
		Use:          "apply -f FILENAME [flags]",
		Short:        "Apply resource manifests to the cluster",
		Long:         "Load AuthProxy resource manifests from files, directories, URLs or stdin. Apply in dependency order, or validate offline with --dry-run=client. Batches are not transactional: independent resources continue after failures.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun != "client" && dryRun != "none" {
				return fmt.Errorf("--dry-run must be none or client")
			}

			if timeout < 0 {
				return fmt.Errorf("--request-timeout cannot be negative")
			}

			if validation == "true" {
				validation = "strict"
			}

			if validation == "false" {
				validation = "ignore"
			}

			options.Validation = apply.Validation(validation)

			var warningErr error
			options.Warn = func(message string) {
				if warningErr == nil {
					_, warningErr = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+message)
				}
			}

			switch output {
			case "", "name", "json", "yaml":
				// Valid
			default:
				return fmt.Errorf("unsupported output format %q", output)
			}

			options.Stdin = cmd.InOrStdin()

			docs, err := apply.Load(cmd.Context(), options)
			if err != nil {
				return err
			}

			if warningErr != nil {
				return warningErr
			}

			if dryRun == "none" && len(docs) > 0 {
				client, err := resolver.ResolveApplyClient(timeout)
				if err != nil {
					return err
				}

				batch, err := client.Prepare(
					cmd.Context(),
					docs,
					apply.ReconcileOptions{Overwrite: overwrite},
				)
				if err != nil {
					return err
				}

				for _, warning := range batch.Warnings() {
					if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+warning); err != nil {
						return err
					}
				}

				results, executionErr := batch.Execute(cmd.Context())

				outputErr := writeApplyResults(cmd, output, results)
				if outputErr != nil {
					outputErr = fmt.Errorf("cannot write apply results; successful writes remain applied: %w", outputErr)
				}

				return errors.Join(executionErr, outputErr)
			}

			// Prepare all output before printing so validation/redaction errors cannot
			// leave a misleading partial batch on stdout.
			objects := make([]any, 0, len(docs))
			for _, doc := range docs {
				value, err := doc.RedactedObject()
				if err != nil {
					return err
				}
				objects = append(objects, value)
			}

			switch output {
			case "json":
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(objects)
			case "yaml":
				encoder := yaml.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent(2)
				for _, object := range objects {
					if err := encoder.Encode(object); err != nil {
						return err
					}
				}
				return encoder.Close()
			default:
				for _, doc := range docs {
					identity := doc.Metadata.ID
					if identity == "" {
						identity = string(doc.Metadata.Name)
						if doc.Metadata.Namespace != "" {
							identity = doc.Metadata.Namespace + "/" + identity
						}
					}
					line := strings.ToLower(string(doc.Kind)) + "/" + identity
					if output == "" {
						line += " validated (client dry run)"
					}
					if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
						return err
					}
				}
				return nil
			}
		},
	}

	cmd.Flags().StringArrayVarP(&options.Filenames, "filename", "f", nil, "File, directory, HTTP(S) URL, or - for stdin (repeatable)")
	cmd.Flags().BoolVarP(&options.Recursive, "recursive", "R", false, "Read manifest directories recursively")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", "", "Default namespace when omitted from a resource")
	cmd.Flags().StringVarP(&options.Selector, "selector", "l", "", "Filter labels using =, ==, !=, key, or !key")
	cmd.Flags().StringVar(&dryRun, "dry-run", "none", "none applies to the cluster; client validates without cluster access")
	cmd.Flags().StringVar(&validation, "validate", "strict", "Unknown-field validation: strict, warn, ignore")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: name, json, yaml (default: operation status)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", true, "Allow overwriting managed-field drift (no effect during client dry-run)")
	cmd.Flags().DurationVar(&timeout, "request-timeout", apply.DefaultRequestTimeout, "Timeout per cluster request; 0 disables the deadline")
	resolver = config.WithConfigParams(cmd)
	return cmd
}

// Structured execution output contains per-resource results, including failures
// and skips. It is emitted after execution; an output error cannot roll back
// successful writes and must never trigger a mutation retry.
func writeApplyResults(cmd *cobra.Command, format string, results []apply.Result) error {
	switch format {
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		return encoder.Encode(results)
	case "yaml":
		encoder := yaml.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent(2)
		for _, result := range results {
			data, err := json.Marshal(result)
			if err != nil {
				return err
			}
			var node yaml.Node
			if err := yaml.Unmarshal(data, &node); err != nil {
				return err
			}
			if err := encoder.Encode(&node); err != nil {
				return err
			}
		}
		return encoder.Close()
	default:
		for _, result := range results {
			if result.Error != "" {
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "%s %s: %s\n", apply.ResultName(result), result.Status, result.Error); err != nil {
					return err
				}
			}
			if format == "name" && (result.Status == "failed" || result.Status == "skipped") {
				continue
			}
			line := apply.ResultName(result)
			if format != "name" {
				line += " " + result.Status
			}
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), line); err != nil {
				return err
			}
		}
		return nil
	}
}
