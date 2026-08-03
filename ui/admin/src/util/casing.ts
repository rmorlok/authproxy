// Sort field values are API enum values and deliberately remain snake_case,
// even though the response fields rendered by the UI are lowerCamelCase.
export function toSnakeCase(value: string): string {
    return value.replace(/[A-Z]/g, (letter) => `_${letter.toLowerCase()}`);
}
