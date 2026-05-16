import { navigate as rawNavigate } from "svelte-routing";

/**
 * Get the configured base path from the Go server's injection.
 * In production, Go injects: window.__GS_BASE_PATH__ = "/gatesentry"
 * In dev (vite), it's undefined → default to ""
 */
export function getBasePath(): string {
    const bp = (window as any).__GS_BASE_PATH__ || "";
    if (bp === "/") return "";
    return bp;
}

/**
 * Base-path-aware navigation. Prepends GS_BASE_PATH to the given path.
 * e.g., gsNavigate("/login") → navigate("/gatesentry/login") when base is "/gatesentry"
 *        gsNavigate("/login") → navigate("/login") when base is "/"
 */
export function gsNavigate(to: string, opts?: { state?: any; replace?: boolean }) {
    const base = getBasePath();
    rawNavigate(base + to, opts);
}
