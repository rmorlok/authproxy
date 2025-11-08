import '@testing-library/jest-dom';
import { expect, vi } from 'vitest';

// Provide a minimal jest shim for libraries/tests that reference `jest`
// eslint-disable-next-line @typescript-eslint/no-explicit-any
(globalThis as any).jest = {
  fn: vi.fn,
  spyOn: vi.spyOn,
  useFakeTimers: vi.useFakeTimers,
  useRealTimers: vi.useRealTimers,
  advanceTimersByTime: vi.advanceTimersByTime,
  runAllTimers: vi.runAllTimers,
};

// Mock matchMedia which is used by some MUI components in tests
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => ({
    matches: false,
    media: query,
    onchange: null,
    addListener: () => {},
    removeListener: () => {},
    addEventListener: () => {},
    removeEventListener: () => {},
    dispatchEvent: () => false,
  }),
});
