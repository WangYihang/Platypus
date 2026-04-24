import { test } from "../fixtures/test";

// Two agents on the same test machine trip the
// hosts.(project_id, fingerprint) UNIQUE constraint — both processes
// derive the same os.Hostname / machine-id so the second Upsert is
// rejected. We can't work around that without a --machine-id CLI
// override on the agent or a fingerprint-override at enrollment.
// Also pending: v2 doesn't surface connected agents as session rows
// yet, so the "two live sessions" assertion wouldn't have data even
// with two hosts.
test.skip("two agents → two live sessions on the project", async () => {});
