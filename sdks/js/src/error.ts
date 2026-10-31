import type { SendEvidence } from './common';

export type ApiError = {
  error: string;
  stackTrace?: string;
  /** Proxy-only metadata. Missing evidence is unknown, including older servers. */
  sendEvidence?: SendEvidence;
};
