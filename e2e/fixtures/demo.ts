import type { Locator, Page } from "@playwright/test";

// Tiny narration helpers for the demo specs under specs/_demo/. They
// inject DOM into the page so the recorded video has captions /
// highlights — viewers can follow what's happening without commentary.
//
// Not used by the regression specs; kept separate so the captions
// don't muddy assertions there.

const CAPTION_ID = "platypus-demo-caption";
const HIGHLIGHT_ID = "platypus-demo-highlight";

// caption pins a banner at the top of the viewport with the supplied
// text and waits `ms` so it's visible long enough to read on the
// playback. Calling it again replaces the text without flicker.
export async function caption(page: Page, text: string, ms = 700) {
    await page.evaluate(
        ({ text, id }) => {
            let el = document.getElementById(id);
            if (!el) {
                el = document.createElement("div");
                el.id = id;
                Object.assign(el.style, {
                    position: "fixed",
                    top: "16px",
                    left: "50%",
                    transform: "translateX(-50%)",
                    background: "rgba(15,15,15,0.92)",
                    color: "#fafafa",
                    padding: "10px 18px",
                    borderRadius: "8px",
                    fontFamily:
                        "var(--font-geist-sans), -apple-system, sans-serif",
                    fontSize: "14px",
                    fontWeight: "500",
                    letterSpacing: "0.1px",
                    boxShadow: "0 8px 24px rgba(0,0,0,0.4)",
                    zIndex: "999999",
                    pointerEvents: "none",
                    transition: "opacity 200ms ease",
                    maxWidth: "min(90vw, 720px)",
                    textAlign: "center",
                });
                document.body.appendChild(el);
            }
            el.textContent = text;
            el.style.opacity = "1";
        },
        { text, id: CAPTION_ID },
    );
    await page.waitForTimeout(ms);
}

// highlight wraps the locator in a soft white outline so viewers can
// see what's about to be clicked. Pure visual — does not click.
export async function highlight(page: Page, locator: Locator, ms = 500) {
    const handle = await locator.elementHandle();
    if (!handle) return;
    await page.evaluate(
        ({ el, id }) => {
            const r = (el as HTMLElement).getBoundingClientRect();
            let ring = document.getElementById(id);
            if (!ring) {
                ring = document.createElement("div");
                ring.id = id;
                Object.assign(ring.style, {
                    position: "fixed",
                    border: "2px solid #fafafa",
                    borderRadius: "8px",
                    boxShadow: "0 0 0 4px rgba(250,250,250,0.25)",
                    zIndex: "999998",
                    pointerEvents: "none",
                    transition: "all 200ms ease",
                });
                document.body.appendChild(ring);
            }
            ring.style.top = `${r.top - 4}px`;
            ring.style.left = `${r.left - 4}px`;
            ring.style.width = `${r.width + 8}px`;
            ring.style.height = `${r.height + 8}px`;
            ring.style.opacity = "1";
        },
        { el: handle, id: HIGHLIGHT_ID },
    );
    await page.waitForTimeout(ms);
    await handle.dispose();
}

// clearOverlays removes the caption + highlight nodes. Call between
// scenes when you want a clean frame.
export async function clearOverlays(page: Page) {
    await page.evaluate(
        ([cid, hid]) => {
            document.getElementById(cid)?.remove();
            document.getElementById(hid)?.remove();
        },
        [CAPTION_ID, HIGHLIGHT_ID],
    );
}

// pause is a thin wrapper around waitForTimeout, named so the demo
// scripts read like a storyboard.
export async function pause(page: Page, ms = 700) {
    await page.waitForTimeout(ms);
}
