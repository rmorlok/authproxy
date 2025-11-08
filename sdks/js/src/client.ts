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
const extractXsrfToken = (headers: Record<string, any>): string | null => {
  if (!headers) return null;
  return headers['x-xsrf-token'] || headers['X-XSRF-TOKEN'] || null;
};

// Function to check if a request should include XSRF token
const shouldIncludeXsrfToken = (config: any): boolean => {
  const url: string | undefined = config?.url;
  if (!url) return true;
  // Do not include XSRF for excluded endpoints
  if (xsrfExcludePaths.has(url)) return false;
  return true;
};

// Wire up interceptors to an axios instance
function attachInterceptors(instance: AxiosInstance) {
  // Response interceptor to extract XSRF tokens
  instance.interceptors.response.use(
    (response) => {
      const token = extractXsrfToken(response.headers as any);
      if (token) xsrfToken = token;
      return response;
    },
    (error) => {
      // Also check for XSRF tokens in error responses
      const token = extractXsrfToken(error?.response?.headers ?? {});
      if (token) xsrfToken = token;
      return Promise.reject(error);
    }
  );

  // Request interceptor to include XSRF tokens
  instance.interceptors.request.use(
    (config) => {
      if (shouldIncludeXsrfToken(config) && xsrfToken) {
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
}

// Export function to manually set XSRF token (useful for testing or manual token management)
export const setXsrfToken = (token: string | null) => {
  xsrfToken = token;
};

// Export function to get current XSRF token (useful for debugging)
export const getXsrfToken = (): string | null => xsrfToken;
