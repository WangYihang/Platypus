// humanizeError maps a thrown Error / rejected promise into a short
// user-facing string suitable for a toast or inline error banner.
//
// The previous default — `toast.error(\`x: \${String(e)}\`)` — surfaced
// raw HTTP shapes like "Error: 403: {\"error\":\"insufficient role\"}"
// or a JS network failure like "TypeError: Failed to fetch". Neither is
// actionable for a non-engineer. Map by the signals we have (status
// code, message shape, error type) into something the user can read
// and act on without opening DevTools.
//
// Anything that doesn't match a known shape falls through to a cleaned
// version of the underlying message — never undefined, never blank.
//
// Pure function: no DOM, no toast invocation, no React imports — call
// sites stay free to render however they like (toast.error, inline,
// alert dialog, etc.).

/** Parsed shape of an HTTP rejection. */
interface ParsedHTTP {
    status: number;
    body: string;
    /** Best-effort `error` string pulled from the JSON body. */
    serverMessage?: string;
}

/**
 * tryParseHTTP recognises errors thrown by lib/auth.ts::authFetch
 * (and the few hand-rolled `${r.status}: ${body}` throws still in
 * lib/api.ts). Returns null if the error is not in that shape.
 */
function tryParseHTTP(e: unknown): ParsedHTTP | null {
    if (!(e instanceof Error)) return null;
    // Match leading "<digits>: <rest>". Status comes first, body is
    // whatever followed it.
    const m = e.message.match(/^(\d{3}):\s*(.*)$/s);
    if (!m) return null;
    const status = Number(m[1]);
    const body = m[2] ?? "";
    let serverMessage: string | undefined;
    try {
        const parsed = JSON.parse(body);
        if (parsed && typeof parsed === "object" && typeof parsed.error === "string") {
            serverMessage = parsed.error;
        }
    } catch {
        // Body wasn't JSON; that's fine.
    }
    return { status, body, serverMessage };
}

/** True if `e` looks like a fetch network failure (DNS, refused, offline). */
function isNetworkError(e: unknown): boolean {
    if (!(e instanceof Error)) return false;
    const msg = e.message;
    return (
        /Failed to fetch/i.test(msg) ||
        /NetworkError/i.test(msg) ||
        /ECONNREFUSED/i.test(msg) ||
        /ENOTFOUND/i.test(msg) ||
        /Load failed/i.test(msg) // Safari's fetch-failure flavour
    );
}

/**
 * humanizeError converts an arbitrary error into a short, user-facing
 * string. Returns:
 *
 *   · 401  "Session expired — please log in again."
 *   · 403  "You don't have permission for this action."
 *   · 404  "That resource no longer exists. Refresh and try again."
 *   · 409  "Conflict: <server message or fallback>."
 *   · 5xx  "Server error (<code>) — try again in a moment."
 *   · network "Cannot reach the server. Check your connection."
 *   · otherwise: the underlying message with leading "Error: "
 *     stripped, which is at least readable.
 */
export function humanizeError(e: unknown): string {
    // Network: caught before HTTP because TypeError("Failed to fetch")
    // is not in the "<status>: ..." shape.
    if (isNetworkError(e)) {
        return "Cannot reach the server. Check your connection.";
    }

    const http = tryParseHTTP(e);
    if (http) {
        switch (http.status) {
            case 401:
                return "Session expired — please log in again.";
            case 403:
                return "You don't have permission for this action.";
            case 404:
                return "That resource no longer exists. Refresh and try again.";
            case 409:
                return http.serverMessage
                    ? `Conflict: ${http.serverMessage}.`
                    : "Conflict: this resource was changed by someone else. Refresh and retry.";
            case 429:
                return "Too many requests — slow down and try again in a moment.";
            default:
                if (http.status >= 500) {
                    return `Server error (${http.status}) — try again in a moment.`;
                }
                if (http.status >= 400 && http.serverMessage) {
                    // Other 4xx with an explicit server-side message:
                    // surface that, since it's been written for the user.
                    return capitalise(http.serverMessage);
                }
                return `Request failed (${http.status}). Try again.`;
        }
    }

    // Specific named errors from auth.ts. We can't import the classes
    // here without a circular dep, so we duck-type by error name.
    if (e instanceof Error) {
        if (e.name === "SessionExpiredError") {
            return "Session expired — please log in again.";
        }
        if (e.name === "StaleServerResponseError") {
            return "You switched servers while this request was loading. Try again.";
        }
        // Strip the "Error: " prefix that String(err) adds in some
        // browsers; trim trailing punctuation so the toast formatter
        // can append its own period.
        return capitalise(e.message.replace(/^Error:\s*/i, "").trim());
    }

    return String(e);
}

function capitalise(s: string): string {
    if (s.length === 0) return s;
    return s.charAt(0).toUpperCase() + s.slice(1);
}
