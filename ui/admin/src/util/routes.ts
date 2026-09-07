export function namespaceDetailPath(path: string): string {
    return `/namespaces/${encodeURIComponent(path)}`;
}
