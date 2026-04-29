import axios, { AxiosInstance, AxiosRequestConfig } from 'axios';

// XSRF token storage
let xsrfToken: string | null = null;

// Paths where XSRF should NOT be sent by default
let xsrfExcludePaths: Set<string> = new Set(['/api/v1/session/_initiate']);

// Shared axios client instance used by the SDK modules
// Initialized with safe defaults; projects should call configureClient(...) to set baseURL, etc.
export let client: AxiosInstance = axios.create({
  timeout: 10000,
  withCredentials: true,
  headers: { Accept: 'application/json' },
});

// Function to extract XSRF token from response headers
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const extractXsrfToken = (headers: Record<string, any>): string | null => {
  if (!headers) return null;
  return headers['x-xsrf-token'] || headers['X-XSRF-TOKEN'] || null;
};

// Function to check if a request should include XSRF token
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const shouldIncludeXsrfToken = (config: any): boolean => {
  const url: string | undefined = config?.url;
  if (!url) return true;
  // Do not include XSRF for excluded endpoints
  if (xsrfExcludePaths.has(url)) return false;
  return true;
};

// Resolve the full request URL from an axios config, joining baseURL + url
// the same way axios does, so log lines show what the browser actually sent.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const resolveRequestUrl = (config: any): string => {
  const url: string = config?.url ?? '';
  const baseURL: string = config?.baseURL ?? '';
  if (!url) return baseURL;
  if (/^https?:\/\//i.test(url) || !baseURL) return url;
  return baseURL.replace(/\/+$/, '') + (url.startsWith('/') ? url : '/' + url);
};

// eslint-disable-next-line @typescript-eslint/no-explicit-any
const logRequestError = (error: any): void => {
  if (typeof console === 'undefined') return;
  const config = error?.config ?? {};
  const method = (config.method ?? 'request').toString().toUpperCase();
  const url = resolveRequestUrl(config);
  const status = error?.response?.status;
  const data = error?.response?.data;
  const pageOrigin =
    typeof window !== 'undefined' && window.location ? window.location.origin : '(non-browser)';

  if (status) {
    console.error(
      `[AuthProxy SDK] ${method} ${url} failed: HTTP ${status}`,
      { status, data, pageOrigin },
    );
    return;
  }

  // No response received — most commonly a CORS preflight rejection or the
  // server being unreachable. The browser does not expose CORS errors to
  // JS, so we have to infer; log enough to make the cause obvious.
  console.error(
    `[AuthProxy SDK] ${method} ${url} failed with no response. ` +
      `This is usually a CORS rejection (the server returned no Access-Control-Allow-Origin ` +
      `for page origin ${pageOrigin}) or the server being unreachable. ` +
      `Check that the server's CORS allow list includes ${pageOrigin}, and that ` +
      `the configured baseURL (${config.baseURL ?? '(unset)'}) is reachable.`,
    { error },
  );
};

// Wire up interceptors to an axios instance
function attachInterceptors(instance: AxiosInstance) {
  // Response interceptor to extract XSRF tokens
  instance.interceptors.response.use(
    (response) => {
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        const token = extractXsrfToken(response.headers as any);
      if (token) xsrfToken = token;
      return response;
    },
    (error) => {
      // Also check for XSRF tokens in error responses
      const token = extractXsrfToken(error?.response?.headers ?? {});
      if (token) xsrfToken = token;
      logRequestError(error);
      return Promise.reject(error);
    }
  );

  // Request interceptor to include XSRF tokens
  instance.interceptors.request.use(
    (config) => {
      if (shouldIncludeXsrfToken(config) && xsrfToken) {
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          (config.headers as any)['X-XSRF-TOKEN'] = xsrfToken;
      }
      return config;
    },
    (error) => Promise.reject(error)
  );
}

// Attach to default instance
attachInterceptors(client);

export type AuthProxyClientOptions = {
  baseURL?: string;
  timeoutMs?: number;
  withCredentials?: boolean;
  defaultHeaders?: Record<string, string>;
  // List of exact URL paths that should NOT receive the XSRF header (e.g. session initiation)
  xsrfExcludePaths?: string[];
  // Additional axios config override if needed
  axiosConfigOverride?: AxiosRequestConfig;
};

/**
 * Configure the shared axios client used by the SDK.
 * It replaces the current instance with a new one built from the options
 * and re-attaches XSRF interceptors.
 */
export function configureClient(opts: AuthProxyClientOptions = {}) {
  const {
    baseURL,
    timeoutMs = 10000,
    withCredentials = true,
    defaultHeaders = { Accept: 'application/json' },
    xsrfExcludePaths: excluded,
    axiosConfigOverride,
  } = opts;

  if (excluded && Array.isArray(excluded)) {
    xsrfExcludePaths = new Set(excluded);
  }

  const config: AxiosRequestConfig = {
    baseURL,
    timeout: timeoutMs,
    withCredentials,
    headers: defaultHeaders,
    ...axiosConfigOverride,
  };

  // Create new instance and attach interceptors
  client = axios.create(config);
  attachInterceptors(client);

  if (typeof console !== 'undefined') {
    const pageOrigin =
      typeof window !== 'undefined' && window.location ? window.location.origin : '(non-browser)';
    console.info(
      `[AuthProxy SDK] client configured: baseURL=${baseURL ?? '(unset)'}, ` +
        `withCredentials=${withCredentials}, page origin=${pageOrigin}`,
    );
  }
}

// Export function to manually set XSRF token (useful for testing or manual token management)
export const setXsrfToken = (token: string | null) => {
  xsrfToken = token;
};

// Export function to get current XSRF token (useful for debugging)
export const getXsrfToken = (): string | null => xsrfToken;
