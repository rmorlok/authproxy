// Configuration for the exponential backoff
export interface BackoffConfig {
    initialDelay: number;  // Initial delay in milliseconds
    maxDelay: number;      // Maximum delay in milliseconds
    maxAttempts: number;   // Maximum number of polling attempts
    backoffFactor: number; // Factor by which the delay increases
}