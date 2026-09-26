package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/cmd/cli/config"
	apply2 "github.com/rmorlok/authproxy/internal/cli/apply"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func cmdApply() *cobra.Command {
	var options apply2.Options
	var dryRun, output, validation string
	var overwrite bool
	var kustomize, expression string
	var allowMissing, prune bool
	var pruneOptions apply2.PruneOptions
	var timeout time.Duration
	var resolver *config.Resolver
	cmd := &cobra.Command{
		Use:          "apply (-f FILENAME | -k DIRECTORY) [flags]",
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

			options.Validation = apply2.Validation(validation)

			var warningErr error
			options.Warn = func(message string) {
				if warningErr == nil {
					_, warningErr = fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+message)
				}
			}

			printer, err := newApplyPrinter(output, expression, allowMissing)
			if err != nil {
				return err
			}

			pruneOptions.Namespace, pruneOptions.Selector = options.Namespace, options.Selector
			if prune {
				if err := pruneOptions.Validate(); err != nil {
					return err
				}

				if dryRun != "none" {
					return fmt.Errorf("--prune requires cluster execution; client dry-run cannot inventory deletions")
				}
			} else if len(pruneOptions.Allowlist) > 0 || pruneOptions.All || cmd.Flags().Changed("wait") || cmd.Flags().Changed("timeout") {
				return fmt.Errorf("--prune-allowlist, --all, --wait and --timeout require --prune")
			}

			options.Stdin = cmd.InOrStdin()

			docs, err := loadApplyInput(cmd, options, kustomize)
			if err != nil {
				return err
			}

			if warningErr != nil {
				return warningErr
			}

			if prune && len(docs) == 0 {
				return fmt.Errorf("refusing prune with no selected manifests")
			}

			if dryRun == "none" && len(docs) > 0 {
				client, err := resolver.ResolveApplyClient(timeout)
				if err != nil {
					return err
				}

				batch, err := client.Prepare(
					cmd.Context(),
					docs,
					apply2.ReconcileOptions{Overwrite: overwrite},
				)
				if err != nil {
					return err
				}

				var prunePlan *apply2.PrunePlan
				if prune {
					prunePlan, err = batch.PreparePrune(cmd.Context(), pruneOptions)
					if err != nil {
						return err
					}
				}

				for _, warning := range batch.Warnings() {
					if _, err := fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+warning); err != nil {
						return err
					}
				}

				results, executionErr := batch.Execute(cmd.Context())

				if executionErr == nil && prunePlan != nil {
					pruned, err := prunePlan.Execute(cmd.Context())
					results = append(results, pruned...)
					executionErr = err
				}

				var outputErr error
				if printer != nil {
					outputErr = printer(cmd.OutOrStdout(), results)
				} else {
					outputErr = writeApplyResults(cmd, output, results)
				}

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

			if printer != nil {
				return printer(cmd.OutOrStdout(), objects)
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
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output: name, json, yaml, go-template[-file], jsonpath[-file], jsonpath-as-json")
	cmd.Flags().BoolVar(&overwrite, "overwrite", true, "Allow overwriting managed-field drift (no effect during client dry-run)")
	cmd.Flags().DurationVar(&timeout, "request-timeout", apply2.DefaultRequestTimeout, "Timeout per cluster request; 0 disables the deadline")
	cmd.Flags().StringVarP(&kustomize, "kustomize", "k", "", "Build a Kustomize directory (exclusive with -f and -R)")
	cmd.MarkFlagsMutuallyExclusive("kustomize", "filename")
	cmd.MarkFlagsMutuallyExclusive("kustomize", "recursive")
	cmd.Flags().StringVar(&expression, "template", "", "Go-template or JSONPath expression, or path with a file output format")
	cmd.Flags().BoolVar(&allowMissing, "allow-missing-template-keys", true, "Ignore missing fields in templates and JSONPath")
	cmd.Flags().BoolVar(&prune, "prune", false, "Delete omitted previously applied resources in an explicit scope")
	cmd.Flags().StringSliceVar(&pruneOptions.Allowlist, "prune-allowlist", nil, "Prunable kinds: Actor, Key, RateLimit (repeatable or comma-separated)")
	cmd.Flags().BoolVar(&pruneOptions.All, "all", false, "Prune all labels within the explicit namespace and kind scope")
	cmd.Flags().BoolVar(&pruneOptions.Wait, "wait", true, "Wait for each pruned resource to return HTTP 404")
	cmd.Flags().DurationVar(&pruneOptions.Timeout, "timeout", 30*time.Second, "Deletion wait timeout per resource; 0 disables the deadline")
	cmd.AddCommand(cmdApplyHistory("view-last-applied"), cmdApplyHistory("set-last-applied"), cmdApplyHistory("edit-last-applied"))
	resolver = config.WithConfigParams(cmd)
	return cmd
}

// Structured execution output contains per-resource results, including failures
// and skips. It is emitted after execution; an output error cannot roll back
// successful writes and must never trigger a mutation retry.
func writeApplyResults(
	cmd *cobra.Command,
	format string,
	results []apply2.Result,
) error {
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
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "%s %s: %s\n", apply2.ResultName(result), result.Status, result.Error); err != nil {
					return err
				}
			}
			if format == "name" && (result.Status == "failed" || result.Status == "skipped") {
				continue
			}
			line := apply2.ResultName(result)
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

func loadApplyInput(
	cmd *cobra.Command,
	options apply2.Options,
	directory string,
) ([]apply2.Document, error) {
	if directory != "" {
		return apply2.LoadKustomize(cmd.Context(), directory, options)
	}
	return apply2.Load(cmd.Context(), options)
}
