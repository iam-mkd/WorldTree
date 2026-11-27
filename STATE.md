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
- Day 4 (`up`; per-world lock; compose-go .env parsing; files resolved once; README; Apache-2.0) — merged (PR #4).
- Day 5 (`down` + `ps`; cleanup-container safety) — merged (PR #5).
- Day 6 (`internal/envfile`: ports, DATABASE_URL, SPRING_DATASOURCE_*, REDIS_URL; `up` writes `.worldtree/env`
  0600; `worldtree env --format dotenv|sh|powershell`) — branch `day-06-env`. Demo passed: host-run Spring jar
  with only `eval "$(worldtree env --format sh)"` wrote into world B, world A empty; PowerShell format loads.

## Workflow
- Repo: https://github.com/iam-mkd/WorldTree (public). Initial commits pushed to `main` on 2026-11-26.
- From now on: every change goes on a branch + PR; never push to `main` directly.

## Next
- Day 7: integration harness (real Docker, build tag `integration`) + ubuntu CI job: two worlds of spring-notes,
  isolation asserted; also exercises `down`'s Linux permission fallback for real.
- Known gap: a port grabbed by a non-worldtree process after leasing makes every re-run of `up` fail on it.

## Open questions
- Licence decided: Apache-2.0 (2026-11-27). Still open: may AGPL/GPL compose files be committed as testdata?
- Installing worktrunk for Day 11 end-to-end test needs approval.
- Unverified on macOS and with real worktrunk. Day 2 ran on Linux (cloud container: Compose v5.7.1, no Docker daemon,
  so only `compose config` was exercised; containers were not started).
