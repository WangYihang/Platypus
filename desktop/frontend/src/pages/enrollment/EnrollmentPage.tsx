import { ShieldCheck, Zap } from "lucide-react";
import { useNavigate, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";

import PageHeader from "../../components/PageHeader";
import { useCurrentProject } from "../../layout/ProjectShell";
import { pendingApprovalCount } from "../../lib/api";
import { qk } from "../../lib/queryKeys";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import ApprovalsPage from "../ApprovalsPage";
import InstallPanel from "./install/InstallPanel";
import PATPanel from "./tokens/PATPanel";

const TABS = ["install", "tokens", "approvals"] as const;
type EnrollmentTab = (typeof TABS)[number];

// EnrollmentPage is the agent-onboarding hub. Three sub-surfaces, all
// admin verbs that share the same mental model ("how do new agents
// get into this project?"):
//
//  1. Install commands: one-shot `curl ... | sh` bootstraps that
//     atomically issue a single-use enrollment token on first fetch.
//  2. Raw enrollment tokens: long-lived credentials for scripted /
//     CI flows that can't pipe to a shell.
//  3. Approvals: queue of fresh agents awaiting an admin go-ahead
//     (anything redeemed without auto_approve lands here).
//
// Day-to-day onboarding ("I want to add a host now") happens through
// the four-step EnrollAgentWizard mounted on HostsShell — this page
// is for issuance management and the approval queue.
//
// The tokens are called "enrollment tokens" on the user surface.
// The backend tables / code paths still use the historical "PAT"
// name (PATPanel etc.) but exposing that acronym led users to confuse
// them with account-scoped API tokens; they are not. They burn on
// first enrollment; mTLS takes over for everything after.
export default function EnrollmentPage() {
    const project = useCurrentProject();
    const navigate = useNavigate();
    const { tab: tabParam } = useParams<{ tab?: string }>();
    const tab: EnrollmentTab = (TABS as readonly string[]).includes(tabParam ?? "")
        ? (tabParam as EnrollmentTab)
        : "install";

    const { data: pendingCount = 0 } = useQuery({
        queryKey: qk.pendingHostsCount(project.id),
        queryFn: () => pendingApprovalCount(project.id),
        refetchInterval: 10_000,
    });

    return (
        <>
            <PageHeader
                title="Enrollment"
                subtitle="Issue install commands or raw tokens, and approve fresh agents waiting to join"
            />
            <Tabs
                value={tab}
                onValueChange={(v) =>
                    navigate(`/projects/${project.slug}/enrollment/${v}`)
                }
                className="px-8 pt-4"
            >
                <TabsList>
                    <TabsTrigger value="install">
                        <Zap className="size-3.5" />
                        Install commands
                    </TabsTrigger>
                    <TabsTrigger value="tokens">Enrollment tokens</TabsTrigger>
                    <TabsTrigger value="approvals">
                        <ShieldCheck className="size-3.5" />
                        Approvals
                        {pendingCount > 0 && (
                            <span
                                aria-label={`${pendingCount} pending`}
                                style={{
                                    marginLeft: 6,
                                    minWidth: 16,
                                    height: 16,
                                    padding: "0 4px",
                                    borderRadius: 999,
                                    background: "var(--color-warning, #b45309)",
                                    color: "#fff",
                                    fontSize: 10,
                                    fontWeight: 600,
                                    display: "inline-flex",
                                    alignItems: "center",
                                    justifyContent: "center",
                                }}
                            >
                                {pendingCount}
                            </span>
                        )}
                    </TabsTrigger>
                </TabsList>
                <TabsContent value="install" className="mt-4">
                    <InstallPanel projectID={project.id} projectSlug={project.slug} />
                </TabsContent>
                <TabsContent value="tokens" className="mt-4">
                    <PATPanel projectID={project.id} />
                </TabsContent>
                <TabsContent value="approvals" className="mt-4">
                    <ApprovalsPage />
                </TabsContent>
            </Tabs>
        </>
    );
}
