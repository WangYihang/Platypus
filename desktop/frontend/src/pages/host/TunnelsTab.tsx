import EmptyState from "../../components/EmptyState";

// TunnelsTab is the per-host inventory of active port forwards. The
// VS-Code-style HostView places it as a peer of Files / Info /
// Sessions / Processes — when an operator wants to know what
// listeners and dynamic SOCKS proxies they have running on this
// host they look here, not in a separate "Tunnels" page.
//
// Phase-4 scope: render the empty placeholder. The active-tunnel
// inventory + "New forward" action depend on a backend API that
// doesn't exist yet (`GET /projects/:slug/hosts/:id/tunnels`,
// `POST /…/tunnels`); when the API ships this component will
// query it via react-query and render rows like:
//
//   L 127.0.0.1:8080 → web-internal:80   4.2 MiB
//   L 127.0.0.1:5432 → db.internal:5432  7.1 MiB
//   D 127.0.0.1:1080 → SOCKS5 → agent    334 KiB
//
// matching the third mockup screenshot.
interface Props {
    projectID: string;
    hostID: string;
}

export default function TunnelsTab({ projectID: _projectID, hostID: _hostID }: Props) {
    return (
        <div className="flex h-full items-center justify-center p-6">
            <EmptyState
                title="No active tunnels"
                description="Open a port forward or SOCKS proxy from this host to map its services into your local laptop. The backend endpoint for live tunnels is still in flight; this panel will list and manage them once it lands."
            />
        </div>
    );
}
