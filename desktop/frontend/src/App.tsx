import { useEffect, useState } from "react";
import { Spin } from "antd";
import Connect from "./pages/Connect";
import Sessions from "./pages/Sessions";
import { ConnectionStatus } from "../wailsjs/go/app/App";
import { EventsOff, EventsOn } from "../wailsjs/runtime/runtime";
import type { app } from "../wailsjs/go/models";
import "./App.css";

function App() {
    const [status, setStatus] = useState<app.ConnectionStatus | null>(null);

    async function refresh() {
        try {
            setStatus(await ConnectionStatus());
        } catch {
            // On startup this may fail if Wails runtime isn't ready yet;
            // the next event tick will retry.
        }
    }

    useEffect(() => {
        refresh();
        EventsOn("app:connection_changed", () => refresh());
        return () => EventsOff("app:connection_changed");
    }, []);

    if (status === null) {
        return (
            <div style={{ display: "flex", justifyContent: "center", padding: 80 }}>
                <Spin size="large" />
            </div>
        );
    }
    return status.connected ? <Sessions /> : <Connect />;
}

export default App;
