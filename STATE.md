# STATE — WorldTree

## Approved / decided (details in DECISIONS.md)
- Spikes A and B done (Windows 11 + Docker Desktop). Reports: `spikes/a/REPORT.md`, `spikes/b/REPORT.md`.
- Scope narrowed: plug into worktrunk hooks; no worktree manager; no DB cloning/slicing in MVP.
- Rewrite = generated compose override (`!reset`/`!override`) from `docker compose config` JSON.
- Shipping: local CLI, single Go binary, GitHub Releases + Homebrew tap. Name `worldtree` is free on Homebrew.
- Docker images used by spikes removed (19.6 GB -> 7.4 GB).

- `ARCHITECTURE.md` + `PLAN.md` approved 2026-11-26 (incl. Windows = best-effort CI build + unit tests).
- Go 1.27.4 installed (official MSI, checksum verified). Lint = gofmt + go vet + staticcheck v0.8.2
  (not golangci-lint: prebuilt binaries lag new Go versions).

## Progress (PLAN.md)
- Day 1 (scaffold, cobra, `worldtree version`, Makefile, CI on 3 OSes, testdata) — merged (PR #1).
- Day 2 (`internal/compose`: plan + render + golden tests + real-compose acceptance test; `worldtree render`)
  — merged (PR #2).
- Day 3 (`internal/registry` JSON + flock + atomic write; `internal/ports` 4-address TCP/UDP probe + hashed-start
  allocator; `render` uses it as a dry run) — branch `day-01-scaffold-54ed7n` (name fixed by the cloud session).

## Workflow
- Repo: https://github.com/iam-mkd/WorldTree (public). Initial commits pushed to `main` on 2026-11-26.
- From now on: every change goes on a branch + PR; never push to `main` directly.

## Next
- Day 4 runs on the user's machine (real Docker), not in the cloud session. Start from `main` after PR #3 merges.
  `up` v1, in this order, each step idempotent:
  1. `Update`: `Put` world (phase `creating`) — before any Docker call.
  2. `LoadConfig` -> `NewPlan`; refuse if `HasErrors()`.
  3. `Update`: `Lease(id, keys, ports.Default())`.
  4. Write `<dir>/.worldtree/compose.override.yml`.
  5. `docker compose -p <project> -f <original> -f <override> up -d --wait`.
  6. `Update`: phase `ready`.
  Demo: `worldtree up` in two copies of `testdata/compose/spring-notes`; both stacks run, ports differ.

## Open questions
- Licence for WorldTree (MIT / Apache-2.0?) and whether AGPL/GPL compose files may be used as testdata.
- Installing worktrunk for Day 11 end-to-end test needs approval.
- Unverified on macOS and with real worktrunk. Day 2 ran on Linux (cloud container: Compose v5.7.1, no Docker daemon,
  so only `compose config` was exercised; containers were not started).
