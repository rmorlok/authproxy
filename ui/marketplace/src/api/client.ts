import axios from "axios";

// XSRF token storage
let xsrfToken: string | null = null;

// Function to extract XSRF token from response headers
const extractXsrfToken = (headers: any): string | null => {
  return headers['x-xsrf-token'] || headers['X-XSRF-TOKEN'] || null;
};

// Function to check if a request should include XSRF token
const shouldIncludeXsrfToken = (config: any): boolean => {
  const url = config.url;
  
  // Don't include XSRF for the session initiate endpoint
  if (url === '/api/v1/session/_initiate') {
    return false;
  }
  
  return true;
};

export const client = axios.create({
    baseURL: import.meta.env.VITE_PUBLIC_BASE_URL,
    timeout: 10000,
    withCredentials: true,
    headers: {
        'Accept': 'application/json'
    },
});

// Response interceptor to extract XSRF tokens
client.interceptors.response.use(
  (response) => {
    const token = extractXsrfToken(response.headers);
    if (token) {
      xsrfToken = token;
    }
    return response;
  },
  (error) => {
    // Also check for XSRF tokens in error responses
    if (error.response) {
      const token = extractXsrfToken(error.response.headers);
      if (token) {
        xsrfToken = token;
      }
    }
    return Promise.reject(error);
  }
);

// Request interceptor to include XSRF tokens
client.interceptors.request.use(
  (config) => {
    if (shouldIncludeXsrfToken(config) && xsrfToken) {
      config.headers['X-XSRF-TOKEN'] = xsrfToken;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Export function to manually set XSRF token (useful for testing or manual token management)
export const setXsrfToken = (token: string | null) => {
  xsrfToken = token;
};

// Export function to get current XSRF token (useful for debugging)
export const getXsrfToken = (): string | null => {
  return xsrfToken;
};