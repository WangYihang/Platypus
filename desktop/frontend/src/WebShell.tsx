import { useCallback, useState } from "react";

import App from "./App";
import WebLogin from "./pages/WebLogin";

const TOKEN_KEY = "platypus.token";

function hasToken(): boolean {
    return !!localStorage.getItem(TOKEN_KEY);
}

// WebShell is the web-mode root. It gates on localStorage — no token →
// <WebLogin>, token present → <App>, which already handles the tabbed
// session UI and calls ConnectionStatus() to see the cached credentials.
export default function WebShell() {
    const [loggedIn, setLoggedIn] = useState(hasToken());

    // WebLogin calls this after webLogin() succeeds.
    const onLoggedIn = useCallback(() => setLoggedIn(true), []);

    if (!loggedIn) {
        return <WebLogin onLoggedIn={onLoggedIn} />;
    }
    return <App />;
}
