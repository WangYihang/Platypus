import { createServer } from "node:net";

import { expect, test } from "../fixtures/test";

// Onboarding step 2 used to surface raw JS error strings ("TypeError:
// Failed to fetch") and left the Continue button enabled, inviting an
// infinite retry loop on a URL that obviously isn't reachable. Both
// behaviours are user-hostile.

// freePort asks the kernel for an unused port and hands it back. The
// spec needs a URL that nothing answers on; it used to hard-code
// 127.0.0.1:31337 and assume that stayed true, which is not something
// a test can assume about the machine it runs on. When the assumption
// broke the probe succeeded, the wizard advanced, and the failure
// surfaced as "element not found" on a later line — a full re-run of
// the suite went into establishing that it wasn't a real regression.
//
// Binding then closing leaves a small window where something else
// could take the port, but "the kernel just told us this was free"
// beats "someone picked a memorable number in 2019".
async function freePort(): Promise<number> {
    return new Promise((resolve, reject) => {
        const srv = createServer();
        srv.on("error", reject);
        srv.listen(0, "127.0.0.1", () => {
            const addr = srv.address();
            const port = typeof addr === "object" && addr !== null ? addr.port : 0;
            srv.close(() => (port ? resolve(port) : reject(new Error("no port assigned"))));
        });
    });
}

test.describe("onboarding probe error UX", () => {
    test("error message is human-readable and Continue is disabled until URL changes", async ({
        page,
    }) => {
        const deadPort = await freePort();

        // Reach /onboarding from a fresh client.
        await page.goto("/");
        await page.evaluate(() => localStorage.clear());
        await page.goto("/onboarding");
        await page.getByTestId("onboarding-get-started").click();

        // Type a URL that nothing's listening on.
        const url = page.getByTestId("onboarding-url");
        await url.fill(`http://127.0.0.1:${deadPort}`);
        const probe = page.getByTestId("onboarding-probe");
        await probe.click();

        // The probe is done when the error appears. Waiting for the
        // button to become enabled — which is what this used to do —
        // waits for something that never happens: on failure it goes
        // straight from `probing` to `blocked`, both disabled. That
        // wait could only ever time out, so it was written with its
        // failure swallowed, which meant every passing run sat here
        // for the full 10s and a genuine hang looked identical.
        const probeError = page.getByTestId("onboarding-probe-error");
        await expect(probeError).toBeVisible({ timeout: 15_000 });

        // The on-screen error must not be a raw JS class name.
        const text = (await page.locator("body").textContent()) ?? "";
        expect(text).not.toMatch(/TypeError/i);
        expect(text).not.toMatch(/Failed to fetch/i);

        // After the probe fails, Continue stays disabled until the URL
        // changes — clicking it again with the same URL just retries.
        await expect(probe).toBeDisabled();

        // Typing a new URL re-enables the button. Nothing probes this
        // one, so it only has to differ from the first.
        await url.fill(`http://127.0.0.1:${deadPort + 1}`);
        await expect(probe).toBeEnabled();
    });
});
