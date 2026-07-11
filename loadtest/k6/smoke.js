import http from 'k6/http';
import { check, group } from 'k6';

const rate = Number(__ENV.K6_RATE || 2);
const preAllocatedVUs = Number(__ENV.K6_PRE_ALLOCATED_VUS || 4);
const maxVUs = Number(__ENV.K6_MAX_VUS || 16);
const duration = __ENV.K6_DURATION || '30s';

const publicUrl = __ENV.AUTHPROXY_PUBLIC_URL || 'http://authproxy-public:8080';
const apiUrl = __ENV.AUTHPROXY_API_URL || 'http://authproxy-api:8081';
const adminApiUrl = __ENV.AUTHPROXY_ADMIN_API_URL || 'http://authproxy-admin-api:8082';
const workerUrl = __ENV.AUTHPROXY_WORKER_URL || 'http://authproxy-worker:8083';
const providerUrl = __ENV.GO_OAUTH2_SERVER_URL || 'http://go-oauth2-server';

export const options = {
  scenarios: {
    smoke: {
      executor: 'constant-arrival-rate',
      rate,
      timeUnit: '1s',
      duration,
      preAllocatedVUs,
      maxVUs,
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.001'],
    http_req_duration: ['p(95)<500'],
  },
};

function expectOK(name, response) {
  check(response, {
    [`${name} status is 200`]: (res) => res.status === 200,
  });
}

export default function () {
  group('authproxy', () => {
    expectOK('public healthz', http.get(`${publicUrl}/healthz`, { tags: { target: 'authproxy-public' } }));
    expectOK('api ping', http.get(`${apiUrl}/ping`, { tags: { target: 'authproxy-api' } }));
    expectOK('admin-api ping', http.get(`${adminApiUrl}/ping`, { tags: { target: 'authproxy-admin-api' } }));
    expectOK('worker ping', http.get(`${workerUrl}/ping`, { tags: { target: 'authproxy-worker' } }));
  });

  group('provider', () => {
    expectOK('provider health', http.get(`${providerUrl}/test/health`, { tags: { target: 'go-oauth2-server' } }));
  });
}
