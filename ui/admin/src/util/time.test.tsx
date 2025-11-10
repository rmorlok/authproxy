import { describe, expect, it } from 'vitest';
import {formatDuration} from "./time";

describe("formatDuration", () => {
    it("should format duration correctly", () => {
        expect(formatDuration(0)).toBe("0ms");
        expect(formatDuration(1)).toBe("1ms");
        expect(formatDuration(60)).toBe("60ms");
        expect(formatDuration(6000)).toBe("6s");
        expect(formatDuration(6500)).toBe("6.5s");
        expect(formatDuration(60000)).toBe("1min");
        expect(formatDuration(3600000)).toBe("1hr");
        expect(formatDuration(86400000)).toBe("1d");
    });
});