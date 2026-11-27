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
  allocator; `render` uses it as a dry run) — merged (PR #3).
- Day 4 (`worldtree up`: analyse -> refuse errors -> register `creating` + lease in one locked update -> write
  override -> `compose up -d --wait` -> `ready`; `compose.DefaultFiles`; render shares `world.Analyse`) — branch
  `day-04-up`. Demo on real Docker passed: 2 copies of spring-notes up, distinct ports, data isolated,
  re-run kept ports (3 s).
  Deviations: analyse runs *before* registering (refused projects leave no record); `.worldtree/.gitignore` = `*`;
  spring-notes Dockerfile is now multi-stage (builds the jar in Docker).

## Workflow
- Repo: https://github.com/iam-mkd/WorldTree (public). Initial commits pushed to `main` on 2026-11-26.
- From now on: every change goes on a branch + PR; never push to `main` directly.

## Next
- Day 5: `worldtree down [dir]` (compose down -v --remove-orphans by project, remove `.worldtree/`, `Remove` from
  registry; idempotent on unknown/half-created worlds) + `worldtree ps`.
- Known gap: if a non-worldtree process grabs a leased port after `up` leased it, re-running `up` reuses the same
  (busy) port and fails again. Fix candidate: on compose "port is already allocated", release that lease and retry.

## Open questions
- Licence decided: Apache-2.0 (2026-11-27). Still open: may AGPL/GPL compose files be committed as testdata?
- Installing worktrunk for Day 11 end-to-end test needs approval.
- Unverified on macOS and with real worktrunk. Day 2 ran on Linux (cloud container: Compose v5.7.1, no Docker daemon,
  so only `compose config` was exercised; containers were not started).
