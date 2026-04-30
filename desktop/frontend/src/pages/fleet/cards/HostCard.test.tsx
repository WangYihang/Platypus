import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";

import HostCard from "./HostCard";
import type { Host } from "../../../lib/api";

// HostCard's three approval modes are the spec's main interest:
// pending shows inline Approve / Reject + a warning pill, approved
// shows the standard card without the action row, rejected adds a
// muted pill but no actions. The 2026-04 IA pulled approvals into
// the card itself so admins don't have to detour through
// /enrollment/approvals for the common single-host case.

function host(extra: Partial<Host> = {}): Host {
    return {
        id: "h1",
        project_id: "p1",
        machine_id: "abcdef0123456789",
        fingerprint: "fp_abcdef0123456789",
        fingerprint_fallback: false,
        hostname: "web-01",
        primary_alias: "",
        os: "linux",
        arch: "amd64",
        first_seen_at: new Date().toISOString(),
        last_seen_at: new Date().toISOString(),
        approval_status: "approved",
        ...extra,
    };
}

describe("<HostCard>", () => {
    it("renders inline Approve + Reject when approval_status is pending", () => {
        render(
            <MemoryRouter>
                <HostCard
                    host={host({ approval_status: "pending" })}
                    approving={false}
                    rejecting={false}
                    onApprove={vi.fn()}
                    onReject={vi.fn()}
                    onOpen={vi.fn()}
                />
            </MemoryRouter>,
        );
        expect(screen.getByTestId("host-card-approve")).toBeInTheDocument();
        expect(screen.getByTestId("host-card-reject")).toBeInTheDocument();
    });

    it("does not render the action row for approved hosts", () => {
        render(
            <MemoryRouter>
                <HostCard
                    host={host({ approval_status: "approved" })}
                    approving={false}
                    rejecting={false}
                    onApprove={vi.fn()}
                    onReject={vi.fn()}
                    onOpen={vi.fn()}
                />
            </MemoryRouter>,
        );
        expect(screen.queryByTestId("host-card-approve")).not.toBeInTheDocument();
        expect(screen.queryByTestId("host-card-reject")).not.toBeInTheDocument();
    });
});
