import axios from "axios";

// XSRF token storage
let xsrfToken: string | null = null;

// Function to extract XSRF token from response headers
const extractXsrfToken = (headers: any): string | null => {
  return headers['x-xsrf-token'] || headers['X-XSRF-TOKEN'] || null;
};

export const client = axios.create({
    baseURL: import.meta.env.VITE_PUBLIC_BASE_URL,
    timeout: 200,
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
    if (xsrfToken) {
      config.headers['X-XSRF-TOKEN'] = xsrfToken;
    }
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);