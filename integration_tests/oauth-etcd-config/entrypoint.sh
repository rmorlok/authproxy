#!/bin/sh
set -e

while ! etcdctl --endpoints="${ETCD_ENDPOINTS:-http://oauth-etcd:2379}" endpoint health >/dev/null 2>&1; do
  sleep 0.5
done

exec etcdctl --endpoints="${ETCD_ENDPOINTS:-http://oauth-etcd:2379}" put /config/go_oauth2_server.json '{
  "Database": {"Type":"postgres","Host":"oauth-postgres","Port":5432,"User":"go_oauth2_server","Password":"","DatabaseName":"go_oauth2_server","MaxIdleConns":5,"MaxOpenConns":5},
  "Oauth": {"AccessTokenLifetime":3600,"RefreshTokenLifetime":1209600,"AuthCodeLifetime":3600},
  "Session": {"Secret":"test_secret","Path":"/","MaxAge":604800,"HTTPOnly":true},
  "IsDevelopment": true
}'
