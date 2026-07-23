import exec from 'k6/execution';
import http from 'k6/http';
import { check, group } from 'k6';
import { Counter, Rate } from 'k6/metrics';
import { SharedArray } from 'k6/data';

const apiUrl = trimTrailingSlash(__ENV.AUTHPROXY_API_URL || 'http://authproxy-api:8081');
const providerUrl = trimTrailingSlash(__ENV.GO_OAUTH2_SERVER_URL || 'http://go-oauth2-server');
const bearerToken = __ENV.AUTHPROXY_BEARER_TOKEN || '';
const proxyMode = (__ENV.K6_PROXY_MODE || 'raw').toLowerCase();
const scenarioName = __ENV.K6_SCENARIO_NAME || `proxy-${proxyMode}`;
const scenarioShape = (__ENV.K6_SCENARIO_SHAPE || 'constant').toLowerCase();
const apiReplicas = __ENV.K6_API_REPLICAS || 'unspecified';
const connectionsFile = __ENV.K6_CONNECTIONS_FILE || './connections.csv';

const rate = numberEnv('K6_RATE', 100);
const timeUnit = __ENV.K6_TIME_UNIT || '1s';
const duration = __ENV.K6_DURATION || '5m';
const preAllocatedVUs = numberEnv('K6_PRE_ALLOCATED_VUS', 100);
const maxVUs = numberEnv('K6_MAX_VUS', 1000);
const requestTimeout = __ENV.K6_REQUEST_TIMEOUT || '30s';

const p95ThresholdMs = numberEnv('K6_P95_THRESHOLD_MS', 1000);
const maxFailedRate = numberEnv('K6_MAX_FAILED_RATE', 0.001);
const max5xxRate = numberEnv('K6_MAX_5XX_RATE', 0.001);

const upstreamStatus = numberEnv('K6_UPSTREAM_STATUS', 200);
const upstreamBytes = numberEnv('K6_UPSTREAM_BYTES', 256);
const upstreamDelayMs = numberEnv('K6_UPSTREAM_DELAY_MS', 0);
const upstreamJitterMs = numberEnv('K6_UPSTREAM_JITTER_MS', 0);
const upstreamBearerPrefix = __ENV.K6_UPSTREAM_BEARER_PREFIX || 'at_';
const upstreamPathPrefix = trimSlashes(__ENV.K6_UPSTREAM_PATH_PREFIX || '/test/load/resource/proxy');
const proxyMethod = (__ENV.K6_PROXY_METHOD || 'GET').toUpperCase();

const proxyRequests = new Counter('proxy_requests');
const proxyUnexpectedStatus = new Counter('proxy_unexpected_status');
const proxy5xxRate = new Rate('proxy_5xx_rate');
const proxyUpstream5xxRate = new Rate('proxy_upstream_5xx_rate');

const connections = new SharedArray('connections', () => parseConnections(open(connectionsFile)));

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  scenarios: {
    proxy: buildScenario(),
  },
  thresholds: {
    http_req_duration: [`p(95)<${p95ThresholdMs}`],
    http_req_failed: [`rate<${maxFailedRate}`],
    dropped_iterations: ['count==0'],
    proxy_5xx_rate: [`rate<${max5xxRate}`],
    proxy_upstream_5xx_rate: [`rate<${max5xxRate}`],
    proxy_unexpected_status: ['count==0'],
  },
};

export function setup() {
  if (!bearerToken) {
    throw new Error('AUTHPROXY_BEARER_TOKEN is required');
  }
  if (proxyMode !== 'raw' && proxyMode !== 'wrapped') {
    throw new Error(`K6_PROXY_MODE must be raw or wrapped, got ${proxyMode}`);
  }
  console.log(JSON.stringify({
    scenario: scenarioName,
    shape: scenarioShape,
    proxy_mode: proxyMode,
    api_replicas: apiReplicas,
    connection_rows: connections.length,
    rate,
    time_unit: timeUnit,
    duration,
  }));
}

export default function () {
  const connection = selectConnection();
  const upstreamUrl = buildUpstreamUrl(connection);

  group(proxyMode, () => {
    const result = proxyMode === 'wrapped'
      ? wrappedProxy(connection, upstreamUrl)
      : { response: rawProxy(connection, upstreamUrl), upstreamStatus: null };
    recordResponse(result.response, result.upstreamStatus);
  });
}

function rawProxy(connection, upstreamUrl) {
  const url = `${apiUrl}/api/v1/connections/${encodeURIComponent(connection.connection_id)}/_proxy_raw`;
  return http.request(proxyMethod, url, null, {
    timeout: requestTimeout,
    headers: {
      Authorization: `Bearer ${bearerToken}`,
      Accept: 'application/octet-stream',
      'X-AuthProxy-Upstream-URL': upstreamUrl,
      'X-AuthProxy-Label': 'loadtest=true',
    },
    tags: requestTags(connection, 'raw'),
  });
}

function wrappedProxy(connection, upstreamUrl) {
  const url = `${apiUrl}/api/v1/connections/${encodeURIComponent(connection.connection_id)}/_proxy`;
  const payload = JSON.stringify({
    url: upstreamUrl,
    method: proxyMethod,
    labels: {
      loadtest: 'true',
      'loadtest.authproxy.io/scenario': scenarioName,
      'loadtest.authproxy.io/mode': 'wrapped',
    },
  });
  const response = http.post(url, payload, {
    timeout: requestTimeout,
    headers: {
      Authorization: `Bearer ${bearerToken}`,
      'Content-Type': 'application/json',
      Accept: 'application/json',
    },
    tags: requestTags(connection, 'wrapped'),
  });
  return {
    response,
    upstreamStatus: wrappedUpstreamStatus(response),
  };
}

function recordResponse(response, wrappedStatus) {
  const upstreamObservedStatus = proxyMode === 'wrapped' ? wrappedStatus : response.status;
  const api5xx = response.status >= 500 && response.status < 600;
  const upstream5xx = upstreamObservedStatus >= 500 && upstreamObservedStatus < 600;
  const expectedStatus = proxyMode === 'wrapped'
    ? response.status === 200 && upstreamObservedStatus === upstreamStatus
    : response.status === upstreamStatus;

  proxyRequests.add(1);
  proxy5xxRate.add(api5xx);
  proxyUpstream5xxRate.add(upstream5xx);
  proxyUnexpectedStatus.add(expectedStatus ? 0 : 1);

  check(response, {
    'authproxy did not return 5xx': (res) => res.status < 500,
    'upstream status matched': () => upstreamObservedStatus === upstreamStatus,
    'wrapped envelope succeeded': (res) => proxyMode !== 'wrapped' || res.status === 200,
  });
}

function wrappedUpstreamStatus(response) {
  if (response.status !== 200) {
    return 0;
  }
  try {
    const body = response.json();
    return Number(body && body.status_code ? body.status_code : 0);
  } catch (err) {
    return 0;
  }
}

function buildScenario() {
  if (scenarioShape === 'spike') {
    return {
      executor: 'ramping-arrival-rate',
      startRate: numberEnv('K6_SPIKE_BASE_RATE', rate),
      timeUnit,
      preAllocatedVUs,
      maxVUs,
      stages: [
        { target: numberEnv('K6_SPIKE_BASE_RATE', rate), duration: __ENV.K6_SPIKE_RAMP_UP || '1m' },
        { target: numberEnv('K6_SPIKE_RATE', rate * 3), duration: __ENV.K6_SPIKE_HOLD || '3m' },
        { target: numberEnv('K6_SPIKE_BASE_RATE', rate), duration: __ENV.K6_SPIKE_RAMP_DOWN || '1m' },
        { target: numberEnv('K6_SPIKE_BASE_RATE', rate), duration: __ENV.K6_SPIKE_RECOVERY || '3m' },
      ],
    };
  }

  return {
    executor: 'constant-arrival-rate',
    rate,
    timeUnit,
    duration,
    preAllocatedVUs,
    maxVUs,
  };
}

function selectConnection() {
  const iteration = exec.scenario.iterationInTest || exec.scenario.iterationInInstance || 0;
  return connections[iteration % connections.length];
}

function buildUpstreamUrl(connection) {
  const path = `${providerUrl}/${upstreamPathPrefix}/${encodeURIComponent(connection.connection_id)}`;
  const params = [
    ['status', String(upstreamStatus)],
    ['bytes', String(upstreamBytes)],
    ['bearer_prefix', upstreamBearerPrefix],
  ];
  if (upstreamDelayMs > 0) {
    params.push(['delay', `${upstreamDelayMs}ms`]);
  }
  if (upstreamJitterMs > 0) {
    params.push(['jitter', `${upstreamJitterMs}ms`]);
  }
  return `${path}?${params.map(([key, value]) => `${key}=${encodeURIComponent(value)}`).join('&')}`;
}

function requestTags(connection, mode) {
  return {
    scenario: scenarioName,
    proxy_mode: mode,
    api_replicas: apiReplicas,
    connector_id: connection.connector_id || 'unknown',
  };
}

function parseConnections(raw) {
  const lines = String(raw).trim().split(/\r?\n/).filter((line) => line.trim() !== '');
  if (lines.length < 2) {
    throw new Error(`${connectionsFile} must contain a header and at least one connection row`);
  }

  const headers = splitCSVLine(lines[0]).map((header) => header.trim());
  if (headers.indexOf('connection_id') === -1) {
    throw new Error(`${connectionsFile} must include a connection_id column`);
  }

  const rows = [];
  for (let i = 1; i < lines.length; i += 1) {
    const values = splitCSVLine(lines[i]);
    const row = {};
    headers.forEach((header, index) => {
      row[header] = values[index] || '';
    });
    if (!row.connection_id) {
      throw new Error(`${connectionsFile} row ${i + 1} is missing connection_id`);
    }
    rows.push(row);
  }

  return rows;
}

function splitCSVLine(line) {
  const fields = [];
  let field = '';
  let quoted = false;

  for (let i = 0; i < line.length; i += 1) {
    const ch = line[i];
    if (ch === '"') {
      if (quoted && line[i + 1] === '"') {
        field += '"';
        i += 1;
      } else {
        quoted = !quoted;
      }
    } else if (ch === ',' && !quoted) {
      fields.push(field);
      field = '';
    } else {
      field += ch;
    }
  }
  fields.push(field);
  return fields;
}

function numberEnv(name, fallback) {
  const raw = __ENV[name];
  if (raw === undefined || raw === '') {
    return fallback;
  }
  const value = Number(raw);
  if (!Number.isFinite(value)) {
    throw new Error(`${name} must be numeric, got ${raw}`);
  }
  return value;
}

function trimTrailingSlash(value) {
  return value.replace(/\/+$/, '');
}

function trimSlashes(value) {
  return value.replace(/^\/+/, '').replace(/\/+$/, '');
}
