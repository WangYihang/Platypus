import {
    copyFileSync,
    existsSync,
    mkdirSync,
    readdirSync,
    statSync,
    writeFileSync,
} from "node:fs";
import * as path from "node:path";

import { REPO_ROOT } from "../fixtures/env";

// build-demos collects the .webm videos Playwright recorded for the
// demo project (specs/_demo/*.demo.ts) and copies them to a stable
// path under docs/demos/. Mirrors the shape of build-gallery.ts so
// reviewers can spot-check on GitHub without cloning.
//
// Each demo spec produces one directory under e2e/test-results/ with
// a name like `_demo-01-onboarding-demo-walk-...-demo`. We match by
// the leading "<NN>-<name>" prefix, normalise to "NN-name.webm", and
// write a README.md that embeds each video.

const TEST_RESULTS = path.join(REPO_ROOT, "e2e", "test-results");
const DEMOS_OUT = path.join(REPO_ROOT, "docs", "demos");

interface DemoMeta {
    title: string;
    caption: string;
}

// Per-demo metadata (title + one-liner). Keys must match the demo
// spec's filename minus `.demo.ts`.
const META: Record<string, DemoMeta> = {
    "01-onboarding": {
        title: "First-run onboarding wizard",
        caption:
            "Fresh client → /onboarding → paste server URL → log in → land in /projects.",
    },
    "02-server-rail": {
        title: "Slack-style server rail",
        caption:
            "Add a second profile, switch with Ctrl+1 / Ctrl+2, rename via the themed Dialog (no native popups).",
    },
    "03-fleet-views": {
        title: "Fleet · Table / Timeline / Graph",
        caption:
            "One Fleet route, three views toggled by URL. Hosts table → Sessions timeline → Mesh topology graph.",
    },
    "04-terminal-persistence": {
        title: "Global terminal drawer survives navigation",
        caption:
            "Open a shell on a host, walk through Activities and Settings, drawer keeps streaming — the bug that drove the refactor.",
    },
    "05-command-palette": {
        title: "Cmd / Ctrl+K command palette",
        caption:
            "Keyboard-first nav: jump between pages, switch project, open a terminal on any host — all from the palette.",
    },
    "06-auth-redesign": {
        title: "Phase-2 auth: opaque session-token lifecycle",
        caption:
            "Login → server returns a single `pst_…` bearer (no JWT pair). Logout revokes it server-side; subsequent requests fail with 401 immediately.",
    },
    "07-marketplace": {
        title: "Plugin Marketplace tab",
        caption:
            "Global Marketplace tab opens, Refresh syncs the SQLite catalog from the platypus-plugins git index, search filters by plugin name, capability chips disclose what each plugin asks for.",
    },
};

const ORDER = [
    "01-onboarding",
    "02-server-rail",
    "03-fleet-views",
    "04-terminal-persistence",
    "05-command-palette",
    "06-auth-redesign",
    "07-marketplace",
];

interface Found {
    key: string;
    src: string;
    bytes: number;
}

// Playwright's per-test directory looks like
// `01-onboarding.demo.ts-walk-first-run-onboarding-wizard-demo`,
// but long titles get truncated to e.g. `04-terminal-persistence.de-…`
// — the `.demo.ts` suffix may be cut. Match by intersecting the
// leading `<NN>-<slug>` with the known ORDER list, using whichever
// known key is the longest prefix of the directory name.
function keyFromDir(dir: string): string | null {
    let best: string | null = null;
    for (const known of ORDER) {
        if (dir.startsWith(known) && (!best || known.length > best.length)) {
            best = known;
        }
    }
    return best;
}

function findVideos(): Found[] {
    if (!existsSync(TEST_RESULTS)) return [];
    const dirs = readdirSync(TEST_RESULTS).filter((d) => /^\d{2}-/.test(d));
    const found: Record<string, Found> = {};
    for (const dir of dirs) {
        const key = keyFromDir(dir);
        if (!key) continue;
        const src = path.join(TEST_RESULTS, dir, "video.webm");
        if (!existsSync(src)) continue;
        const bytes = statSync(src).size;
        // If multiple runs left several copies, keep the largest
        // (= most footage / most likely to be a complete take).
        const prev = found[key];
        if (!prev || bytes > prev.bytes) {
            found[key] = { key, src, bytes };
        }
    }
    return ORDER.map((k) => found[k]).filter(Boolean) as Found[];
}

function humanBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

function build() {
    if (!existsSync(DEMOS_OUT)) mkdirSync(DEMOS_OUT, { recursive: true });

    const found = findVideos();
    if (found.length === 0) {
        console.error(
            "build-demos: no demo videos found under e2e/test-results/." +
                " Run `pnpm run demos` from e2e/ first.",
        );
        process.exit(1);
    }

    const lines: string[] = [];
    lines.push("# Platypus UI demos");
    lines.push("");
    lines.push(
        "Auto-generated from `e2e/specs/_demo/*.demo.ts`. Re-run with " +
            "`pnpm run demos` from `e2e/`. Each clip is a Playwright " +
            "spec played at slowMo=250 with in-page captions.",
    );
    lines.push("");

    for (const f of found) {
        const dst = path.join(DEMOS_OUT, `${f.key}.webm`);
        copyFileSync(f.src, dst);
        const meta = META[f.key];
        lines.push(`## ${meta.title}`);
        lines.push("");
        lines.push(meta.caption);
        lines.push("");
        lines.push(`<video src="${f.key}.webm" controls width="900"></video>`);
        lines.push("");
        lines.push(`_${humanBytes(f.bytes)} · WebM (VP8/Opus)_`);
        lines.push("");
        console.log(`copied ${f.key}.webm (${humanBytes(f.bytes)})`);
    }

    const outPath = path.join(DEMOS_OUT, "README.md");
    writeFileSync(outPath, lines.join("\n"), "utf8");
    console.log(`wrote ${outPath} (${lines.length} lines)`);
}

build();
