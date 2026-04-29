import { useState } from "react";
import { Zap } from "lucide-react";

import PageHeader from "../../components/PageHeader";
import { useCurrentProject } from "../../layout/ProjectShell";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

import InstallPanel from "./install/InstallPanel";
import PATPanel from "./tokens/PATPanel";

// EnrollmentPage is the historical management surface for two related
// admin verbs:
//
//  1. Install commands: one-shot `curl ... | sh` bootstraps that
//     atomically issue a single-use enrollment token on first fetch.
//  2. Raw enrollment tokens: long-lived credentials for scripted /
//     CI flows that can't pipe to a shell.
//
// Day-to-day onboarding ("I want to add a host now") happens through
// the four-step EnrollAgentWizard inside Fleet card view. This page
// is for the rarer "list / revoke / audit historical issuance" flows
// — which is why it lives under Audit → Enrollment, not under
// Fleet, after the 2026-04 IA pass.
//
// The tokens are called "enrollment tokens" on the user surface.
// The backend tables and code paths still use the historical "PAT"
// name (PATPanel etc.) but exposing that acronym led users to confuse
// them with account-scoped API tokens; they are not. They burn on
// first enrollment; mTLS takes over for everything after.
export default function EnrollmentPage() {
    const project = useCurrentProject();
    const [tab, setTab] = useState<"install" | "tokens">("install");

    return (
        <>
            <PageHeader
                title="Enrollment"
                subtitle="Generate one-shot install commands (recommended) or raw enrollment tokens for CI / automation"
            />
            <Tabs
                value={tab}
                onValueChange={(v) => setTab(v as "install" | "tokens")}
                className="px-8 pt-4"
            >
                <TabsList>
                    <TabsTrigger value="install">
                        <Zap className="size-3.5" />
                        Install commands
                    </TabsTrigger>
                    <TabsTrigger value="tokens">Enrollment tokens</TabsTrigger>
                </TabsList>
                <TabsContent value="install" className="mt-4">
                    <InstallPanel projectID={project.id} projectSlug={project.slug} />
                </TabsContent>
                <TabsContent value="tokens" className="mt-4">
                    <PATPanel projectID={project.id} />
                </TabsContent>
            </Tabs>
        </>
    );
}
