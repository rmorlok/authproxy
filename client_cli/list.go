package main

import "github.com/spf13/cobra"

func cmdList() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List entities",
	}

	cmd.AddCommand(cmdListDomains())

	return cmd
}

func cmdListDomains() *cobra.Command {
	var noBanner bool

	cmd := &cobra.Command{
		Use:   "domains",
		Short: "List domains ",
		RunE: func(cmd *cobra.Command, args []string) error {
			println("TODO")
			return nil
		},
	}

	cmd.Flags().BoolVar(&noBanner, "no-banner", false, "Don't show banner")

	return cmd
}
