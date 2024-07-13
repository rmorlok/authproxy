package main

import (
	"github.com/rmorlok/authproxy/server"
	"github.com/spf13/cobra"
)

func main() {
	var cmdRoutes = &cobra.Command{
		Use:   "routes",
		Short: "Print routes exposed by app",
		Run: func(cmd *cobra.Command, args []string) {
			server.PrintRoutes(server.GetGinServer())
		},
	}

	var cmdServe = &cobra.Command{
		Use:   "serve",
		Short: "Start the server",
		Run: func(cmd *cobra.Command, args []string) {
			server.Serve()
		},
	}

	var rootCmd = &cobra.Command{Use: "kodenames2"}
	rootCmd.AddCommand(cmdRoutes)
	rootCmd.AddCommand(cmdServe)
	rootCmd.Execute()
}
