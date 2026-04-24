import { test } from "../fixtures/test";

// Needs two agents on the same test machine — blocked by the
// hosts.(project_id, fingerprint) UNIQUE constraint. Both processes
// read the same os.Hostname() / machine-id, so the second enroll's
// Upsert hits the existing host row from the first enroll and fails
// with "UNIQUE constraint failed". Unskip once the agent gains a
// --machine-id CLI override (or the server relaxes the uniqueness
// to support explicit re-enrollment).
test.skip("mesh topology: two agents form a mesh link and it appears in the UI", async () => {});
