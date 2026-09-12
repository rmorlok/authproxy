package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rmorlok/authproxy/internal/apply"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func cmdApply() *cobra.Command {
	var options apply.Options
	var dryRun, output, validation string
	var overwrite bool
	cmd := &cobra.Command{
		Use:   "apply -f FILENAME [flags]",
		Short: "Validate resource manifests with client dry-run",
		Long:  "Load AuthProxy resource manifests from files, directories, URLs or stdin. This initial implementation supports --dry-run=client only; cluster writes are not yet available.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRun != "client" {
				return fmt.Errorf("only --dry-run=client is currently supported; cluster apply is not yet available")
			}
			if validation == "true" {
				validation = "strict"
			}
			if validation == "false" {
				validation = "ignore"
			}
			options.Validation = apply.Validation(validation)
			options.Warn = func(message string) { fmt.Fprintln(cmd.ErrOrStderr(), "Warning: "+message) }
			switch output {
			case "", "name", "json", "yaml":
			default:
				return fmt.Errorf("unsupported output format %q", output)
			}
			options.Stdin = cmd.InOrStdin()
			docs, err := apply.Load(cmd.Context(), options)
			if err != nil {
				return err
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
	cmd.Flags().StringVar(&dryRun, "dry-run", "none", "Currently requires client; does not contact the cluster")
	cmd.Flags().StringVar(&validation, "validate", "strict", "Unknown-field validation: strict, warn, ignore")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format: name, json, yaml (default: validation status)")
	cmd.Flags().BoolVar(&overwrite, "overwrite", true, "Allow overwriting managed-field drift (no effect during client dry-run)")
	return cmd
}
