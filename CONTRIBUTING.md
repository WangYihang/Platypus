# Contributing

Thanks for looking. This file covers the parts that aren't obvious from
reading the tree: which checks gate a change, how the repo is laid out,
and what a reviewable pull request looks like here.

Setup and build instructions live in the [README](./README.md#build-from-source);
`make help` lists every target.

## Before you open a pull request

Install the git hooks once:

```bash
pip install pre-commit   # or: pipx install pre-commit
make hooks
```

They run `gofmt`, `goimports`, `go mod tidy`, `golangci-lint`, and
`oxlint` on the frontend. `make pre-commit` runs them against the whole
tree instead of just your staged files.

Then, for whichever part you touched:

| You changed | Run |
| --- | --- |
| Go (server / agent / CLI) | `make test` — race detector on |
| Go lint | `make lint` |
| Desktop Go | `make desktop-test` |
| Frontend | `cd desktop/frontend && pnpm test && pnpm exec tsc --noEmit && pnpm run lint` |
| Anything user-visible | `make e2e-deps` once, then `make e2e` |
| Protobuf | `make proto`, and commit the regenerated files |
| Swagger annotations | `make swag`, and commit `docs/swagger.{yaml,json}` |

CI runs the same things. Nothing here needs credentials or a network
service — the e2e suite spins up its own server and agent.

## What the linters are set to catch

Both linters are deliberately tuned to "real bugs, not style nits", and
both configs say why in comments. Read them before adding a suppression.

**Go** — `.golangci.yml`. `govet`, `staticcheck`, `ineffassign`,
`unconvert`, `unused`, `errcheck`, `misspell`, `gosec`. Three gosec
rules are off repo-wide with reasons in the config; everything else it
reports is either fixed or carries a `//nolint:gosec` naming the rule
and the justification. If you need a new suppression, write the reason
— `//nolint` with no explanation will be asked about in review.

**Frontend** — `desktop/frontend/.oxlintrc.json`. oxlint, not ESLint:
typescript-eslint does not load against TypeScript 7, which this project
is on. `pnpm run lint` runs it with `--type-aware`, which adds the rules
that need the whole program — `no-floating-promises`,
`no-base-to-string` and friends — via oxlint-tsgolint. Everything it
reports is an error and blocks.

That pass needs to build the program, so it takes seconds rather than
milliseconds. Bare `pnpm exec oxlint` gives you the fast parser-only
subset while editing; CI and pre-commit run the type-aware one.

There is no warning backlog: `pnpm run lint:strict`, which treats
warnings as errors, passes too. If you need to silence something,
`oxlint-disable-next-line <rule>` with a comment saying why — a bare
suppression will be asked about in review, same as `//nolint` on the
Go side. Note the comment has to sit on the line the rule reports,
which for a hook dependency complaint is the `}, [deps]);` line rather
than the `useEffect(` line.

## Layout

```
cmd/            server, agent, and CLI entrypoints
internal/       the actual implementation — see below
pkg/            code intended to be importable from outside
proto/          .proto sources; generated Go lands in pkg/proto
desktop/        Wails app; its own go.mod, frontend/ is the React app
e2e/            Playwright suite, runs against a real server + agent
examples/       sample plugins
docs/           Docusaurus site + generated swagger
scripts/        build-time staging tools, not shipped
```

Inside `internal/`, the pieces you're most likely to want:
`agent/` (agent-side, including the WASM plugin host and its capability
sandbox), `core/` (server-side session and link handling), `api/` (HTTP
handlers), `storage/` (sqlite repos), `mesh/` (peer-to-peer links),
`link/` (the yamux framing both ends share).

## Commits and pull requests

Commit subjects follow [Conventional Commits](https://www.conventionalcommits.org/):
`feat(recordings):`, `fix(agent):`, `build(deps):`, `docs:`, `test(api):`.
The scope is optional but helps.

For the body, prefer explaining *why* over restating the diff. A reader
six months out can see what changed; what they can't recover is what
went wrong, what you ruled out, and what you decided not to do.

Keep a pull request to one concern. If you find an unrelated bug while
you're in there, that's a second PR — it makes both easier to review and
to revert.

## Tests

New behaviour needs a test. Bug fixes need one that fails without the
fix; if you can't write one, say so in the PR and explain why.

Go tests live next to the code. Frontend tests are Vitest +
Testing Library, also next to the component. The e2e suite is for flows
that cross the server/agent/UI boundary — it's slower and more brittle,
so prefer a unit test when one will do.

## Security

Don't open a public issue for a vulnerability. [SECURITY.md](./SECURITY.md)
explains what's in scope and how to report privately.

Worth knowing before you change agent- or plugin-side code: the plugin
sandbox's filesystem allowlist (`internal/agent/plugin/host_fs.go`,
`host_fs_write.go`) is a real security boundary, and so is enrollment
and the mTLS setup between agent and server. Changes there get read
closely, and they need tests.
