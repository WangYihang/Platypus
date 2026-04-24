import { test } from "@playwright/test";

// Same blocker as 15/17: two zero-config agents on the same test
// machine collide on hosts.(project_id, fingerprint). Unskip once
// the agent supports a --machine-id override or a fingerprint salt.
test.skip("mesh auto-discovery: two zero-config agents bootstrap via mDNS", async () => {});
