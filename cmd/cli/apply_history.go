package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/rmorlok/authproxy/cmd/cli/config"
	apply2 "github.com/rmorlok/authproxy/internal/cli/apply"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func cmdApplyHistory(action string) *cobra.Command {
	var options apply2.Options
	var directory, output string
	var create bool
	var timeout time.Duration
	var resolver *config.Resolver
	cmd := &cobra.Command{Use: action + " -f FILENAME", Short: strings.ReplaceAll(action, "-", " ") + " without changing resource fields", Args: cobra.NoArgs, SilenceUsage: true}
	cmd.RunE = func(cmd *cobra.Command, args []string) (returnErr error) {
		if output != "yaml" && output != "json" {
			return fmt.Errorf("history output must be yaml or json")
		}
		options.Stdin = cmd.InOrStdin()
		options.Validation = apply2.ValidationStrict
		docs, err := loadApplyInput(cmd, options, directory)
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			return fmt.Errorf("no selected history targets")
		}
		client, err := resolver.ResolveApplyClient(timeout)
		if err != nil {
			return err
		}
		targets, err := client.HistoryTargets(cmd.Context(), docs)
		if err != nil {
			return err
		}
		if action == "set-last-applied" {
			results, err := client.SetLastApplied(cmd.Context(), targets, docs, create)
			if printErr := writeApplyResults(cmd, output, results); printErr != nil {
				return printErr
			}
			return err
		}
		objects := make([]any, 0, len(targets))
		for _, target := range targets {
			desired, err := target.LastApplied()
			if err != nil {
				return err
			}
			objects = append(objects, desired)
		}
		var data bytes.Buffer
		encoder := yaml.NewEncoder(&data)
		encoder.SetIndent(2)
		for _, object := range objects {
			if err := encoder.Encode(object); err != nil {
				return err
			}
		}
		if err := encoder.Close(); err != nil {
			return err
		}
		if action == "view-last-applied" {
			if output == "json" {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(objects)
			}
			_, err = data.WriteTo(cmd.OutOrStdout())
			return err
		}
		file, err := os.CreateTemp("", "ap-last-applied-*.yaml")
		if err != nil {
			return err
		}
		path := file.Name()
		defer func() {
			if returnErr != nil {
				returnErr = fmt.Errorf("%w; edited history retained at %s", returnErr, path)
			} else {
				os.Remove(path)
			}
		}()
		if _, err = file.Write(data.Bytes()); err != nil {
			file.Close()
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
		editor := os.Getenv("KUBE_EDITOR")
		if editor == "" {
			editor = os.Getenv("EDITOR")
		}
		if editor == "" {
			editor = "vi"
		}
		// The editor variable is intentionally a command (it may include flags).
		edit := exec.CommandContext(cmd.Context(), "sh", "-c", editor+` "$1"`, "ap-editor", path)
		edit.Stdin, edit.Stdout, edit.Stderr = os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr()
		if err = edit.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}
		edited, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Equal(edited, data.Bytes()) {
			return nil
		}
		editedDocs, err := apply2.Load(cmd.Context(), apply2.Options{Filenames: []string{"-"}, Stdin: bytes.NewReader(edited), Validation: apply2.ValidationStrict})
		if err != nil {
			return err
		}
		results, err := client.SetLastApplied(cmd.Context(), targets, editedDocs, false)
		if printErr := writeApplyResults(cmd, output, results); printErr != nil {
			return printErr
		}
		return err
	}
	cmd.Flags().StringArrayVarP(&options.Filenames, "filename", "f", nil, "Manifest files identifying existing resources")
	cmd.Flags().StringVarP(&directory, "kustomize", "k", "", "Kustomize directory identifying existing resources")
	cmd.Flags().BoolVarP(&options.Recursive, "recursive", "R", false, "Read directories recursively")
	cmd.MarkFlagsMutuallyExclusive("kustomize", "filename")
	cmd.MarkFlagsMutuallyExclusive("kustomize", "recursive")
	cmd.Flags().StringVarP(&options.Namespace, "namespace", "n", "", "Default namespace when omitted")
	cmd.Flags().StringVarP(&options.Selector, "selector", "l", "", "Select manifest labels")
	cmd.Flags().StringVarP(&output, "output", "o", "yaml", "Output: yaml or json")
	cmd.Flags().DurationVar(&timeout, "request-timeout", apply2.DefaultRequestTimeout, "Timeout per cluster request")
	if action == "set-last-applied" {
		cmd.Flags().BoolVar(&create, "create-annotation", false, "Initialize history on previously unmanaged resources")
	}
	resolver = config.WithConfigParams(cmd)
	return cmd
}
