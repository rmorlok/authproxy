package seeder

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func WriteArtifacts(runDir string, result *Result) error {
	if runDir == "" {
		return nil
	}
	if result == nil {
		return fmt.Errorf("seed result is required")
	}

	datasetsDir := filepath.Join(runDir, "datasets")
	if err := os.MkdirAll(datasetsDir, 0o755); err != nil {
		return err
	}

	if err := writeConnectionsCSV(filepath.Join(datasetsDir, "connections.csv"), result.Connections); err != nil {
		return err
	}
	if err := writeNamespacesCSV(filepath.Join(datasetsDir, "namespaces.csv"), result.Namespaces); err != nil {
		return err
	}
	if err := writeActorsCSV(filepath.Join(datasetsDir, "actors.csv"), result.Actors); err != nil {
		return err
	}

	summary, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(runDir, "seed-summary.json"), append(summary, '\n'), 0o644); err != nil {
		return err
	}

	plan := fmt.Sprintf(`AuthProxy load-test seed summary

Profile: %s
Provider base URL: %s
Base namespace: %s
Connector: %s:%d

Tenant namespaces requested: %d
Connections requested: %d
Connections created: %d
Connections already present: %d
OAuth2 tokens upserted: %d
OAuth expiring percent: %d
Periodic probe percent: %d
Probe-enabled connections: %d
Verified samples: %d

Datasets:
  datasets/connections.csv
  datasets/namespaces.csv
  datasets/actors.csv
`,
		result.ProfileName,
		result.ProviderBaseURL,
		result.BaseNamespace,
		result.ConnectorID,
		result.ConnectorVersion,
		result.RequestedTenantNamespaces,
		result.RequestedConnections,
		result.CreatedConnections,
		result.ExistingConnections,
		result.UpsertedOAuthTokens,
		result.OAuthExpiringPercent,
		result.PeriodicProbePercent,
		result.ProbeEnabledConnections,
		len(result.VerifiedSamples),
	)
	return os.WriteFile(filepath.Join(runDir, "seed-plan.txt"), []byte(plan), 0o644)
}

func writeConnectionsCSV(path string, rows []ConnectionRecord) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{
		"connection_id",
		"namespace",
		"actor_id",
		"connector_id",
		"connector_version",
		"refresh_token",
		"access_token",
		"access_token_expires_at",
		"probe_enabled",
		"oauth_token_id",
	}); err != nil {
		return err
	}
	for _, row := range rows {
		tokenID := ""
		if row.OAuthTokenID != nil {
			tokenID = row.OAuthTokenID.String()
		}
		if err := writer.Write([]string{
			row.ConnectionID.String(),
			row.Namespace,
			row.ActorID.String(),
			row.ConnectorID.String(),
			strconv.FormatUint(row.ConnectorVersion, 10),
			row.RefreshToken,
			row.AccessToken,
			row.AccessTokenExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
			strconv.FormatBool(row.ProbeEnabled),
			tokenID,
		}); err != nil {
			return err
		}
	}
	return writer.Error()
}

func writeNamespacesCSV(path string, rows []NamespaceRecord) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{"namespace"}); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writer.Write([]string{row.Namespace}); err != nil {
			return err
		}
	}
	return writer.Error()
}

func writeActorsCSV(path string, rows []ActorRecord) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	if err := writer.Write([]string{"actor_id", "namespace", "external_id"}); err != nil {
		return err
	}
	for _, row := range rows {
		if err := writer.Write([]string{row.ActorID.String(), row.Namespace, row.ExternalID}); err != nil {
			return err
		}
	}
	return writer.Error()
}
