/**
 * Sends each viewer request to the docs homepage. A temporary, non-cacheable
 * response lets browsers reach the website as soon as the apex is repurposed.
 */
function handler() {
    return {
        statusCode: 302,
        headers: {
            location: { value: "https://docs.authproxy.net/" },
            "cache-control": { value: "no-store" },
        },
    };
}
