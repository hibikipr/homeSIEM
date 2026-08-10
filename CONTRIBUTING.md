# Contributing to homeSIEM

homeSIEM is a homelab project first and foremost — it's built to run one
specific stack well, not to be everything to everyone. That said, bug
reports, fixes, and features that fit the existing design are welcome.

## Before you start

For anything beyond a small fix, open an issue first describing what you
want to change and why. This project has a fairly opinionated architecture
(see each service's README, and `docs/superpowers/specs/` for the larger
features' design docs) - a quick issue discussion saves you from building
something that doesn't fit before you've invested the time.

## Getting set up

Each service has its own local dev instructions:

- `siem-api`: `cd siem-api && go test ./... && go run ./cmd/siem-api` (needs
  the env vars listed in the root `.env.example`).
- [`siem-web/README.md`](siem-web/README.md) - SvelteKit, `pnpm install`,
  `pnpm run dev`.
- [`siem-ingest/README.md`](siem-ingest/README.md) - Vector config, plus
  `siem-ingest/docs/` for GeoIP/threat-intel/TLS setup.

The root [`docker-compose.yml`](README.md#quickstart-docker-composeyml) brings
up a self-contained stack (bundles its own Loki/ntfy) if you want to run
everything together rather than one service at a time.

## Before opening a pull request

- **siem-api (Go):** `go build ./... && go vet ./... && gofmt -l .` should be
  clean, and `go test ./...` should pass. New behavior gets a test - this
  codebase is TDD-style throughout (see any `internal/*/*_test.go` for the
  existing pattern: table-driven tests, local unexported fakes per test file,
  no shared mocking library).
- **siem-web (TypeScript/Svelte):** `pnpm run check` (svelte-check),
  `pnpm run lint` (prettier + eslint), and `pnpm exec vitest run` should all
  be clean. There's no component-test infrastructure for `.svelte` files by
  design - verify those via `svelte-check` + a real `pnpm run build` (and
  manual testing in a browser if you can) rather than adding one.
- Keep changes scoped. A bug fix doesn't need an accompanying refactor; a new
  feature doesn't need to solve problems it wasn't asked to solve.

## Commit / PR conventions

- Small, reviewable PRs over one large one where reasonable.
- Explain *why* in the PR description, not just what changed - the diff
  already shows what changed.
- CI builds and publishes images from GitHub Releases, not every push to
  `main` - see the "Building and publishing images" section of the root
  README if your change needs a release to actually reach anyone running the
  published images.

## Reporting bugs / security issues

Open a GitHub issue for bugs. This is a homelab tool without a formal
security disclosure process - if you find something sensitive (e.g. an auth
bypass), please still just open an issue, but flag it clearly as a security
concern in the title so it doesn't get lost.
