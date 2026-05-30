package main

import (
	"os"

	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/rmorlok/authproxy/plugins/grafana/authproxy-datasource/pkg/plugin"
)

func main() {
	if err := datasource.Manage("rmorlok-authproxy-datasource", plugin.NewDatasource, datasource.ManageOpts{}); err != nil {
		_, _ = os.Stderr.WriteString(err.Error())
		os.Exit(1)
	}
}
