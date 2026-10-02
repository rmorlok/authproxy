//go:build integration

package apply_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rmorlok/authproxy/integration_tests/helpers"
	"github.com/rmorlok/authproxy/internal/apauth/jwt"
	"github.com/rmorlok/authproxy/internal/config"
	"github.com/rmorlok/authproxy/internal/schema/common"
	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Exercise the actual serve command, including local authentication and readiness.
func (c cli) startupApply(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	directory := t.TempDir()
	root := *c.env.Cfg.GetRoot()
	if root.Database.GetProvider() == sconfig.DatabaseProviderSqlite {
		// Give the subprocess its own durable snapshot rather than sharing live WAL
		// handles with the test process. Include schema, keys, and the apply actor.
		snapshot := filepath.Join(directory, "startup.sqlite3")
		_, err = c.env.DM.GetSQLDB().Exec("VACUUM INTO '" + strings.ReplaceAll(snapshot, "'", "''") + "'")
		require.NoError(t, err)
		root.Database = &sconfig.Database{InnerVal: &sconfig.DatabaseSqlite{Provider: sconfig.DatabaseProviderSqlite, Path: snapshot}}
	}
	serviceID := sconfig.ServiceIdApi
	root.Api.PortVal = common.NewIntegerValueDirect(int64(port))
	root.Api.HealthCheckPortVal = nil
	if c.admin {
		serviceID = sconfig.ServiceIdAdminApi
		root.AdminApi.PortVal = common.NewIntegerValueDirect(int64(port))
		root.AdminApi.HealthCheckPortVal = nil
	}
	configBytes, err := json.Marshal(&root)
	require.NoError(t, err)
	// Omit optional runtime-only nil/default values from the serialized config.
	var serialized map[string]any
	require.NoError(t, json.Unmarshal(configBytes, &serialized))
	omitNilConfigFields(serialized)
	delete(serialized["systemAuth"].(map[string]any), "jwtTokenDuration")
	configBytes, err = yaml.Marshal(serialized)
	require.NoError(t, err)
	configPath := filepath.Join(directory, "server.yaml")
	require.NoError(t, os.WriteFile(configPath, configBytes, 0600))
	_, err = config.LoadConfig(configPath)
	require.NoError(t, err)
	doc := manifest("Connector", "startup", "root", object{"definition": object{
		"displayName": "Startup applied", "logo": object{"publicUrl": "https://example.com/logo.png"}, "auth": object{"type": "no-auth"},
	}})
	path := filepath.Join(directory, "resources.yaml")
	require.NoError(t, os.WriteFile(path, []byte(encode(t, doc)), 0600))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.serverBinary, "serve", "--no-banner", "--config="+configPath, "--apply="+path, "--apply-actor=apply-operator", "--apply-timeout=30s", string(serviceID))
	cmd.Dir = c.root
	logPath := filepath.Join(directory, "server.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	defer logFile.Close()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait(); close(done) }()
	// Stop the subprocess before cancelling its context or removing its files.
	defer func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
waiting:
	for {
		select {
		case err := <-done:
			data, _ := os.ReadFile(logPath)
			t.Fatalf("server exited before apply completed: %v\n%s", err, data)
		case <-ctx.Done():
			data, _ := os.ReadFile(logPath)
			t.Fatalf("startup apply timed out: %s", data)
		case <-ticker.C:
			data, _ := os.ReadFile(logPath)
			if strings.Contains(string(data), "create created") {
				break waiting
			}
		}
	}
	key, err := root.SystemAuth.GlobalAESKey.GetCurrentVersion(context.Background())
	require.NoError(t, err)
	token, err := jwt.NewJwtTokenBuilder().WithSystemSigned().WithSecretKey(key.Data).
		WithServiceId(serviceID).WithActorExternalId("apply-operator").WithNamespace("root").WithExpiresIn(time.Minute).Token()
	require.NoError(t, err)
	observer := cli{env: &helpers.IntegrationTestEnv{ServerURL: fmt.Sprintf("http://localhost:%d", port), BearerToken: token}}
	result := observer.request(t, "GET", "connectors?namespace=root&name=startup", nil, 200)
	require.Len(t, result["items"], 1)
}

func omitNilConfigFields(value any) {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if child == nil {
				delete(v, key)
			} else {
				omitNilConfigFields(child)
			}
		}
	case []any:
		for _, child := range v {
			omitNilConfigFields(child)
		}
	}
}
