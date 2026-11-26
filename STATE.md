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
- Day 1 (scaffold, cobra, `worldtree version`, Makefile, CI on 3 OSes, testdata) — PR `day-01-scaffold`.

## Workflow
- Repo: https://github.com/iam-mkd/WorldTree (public). Initial commits pushed to `main` on 2026-11-26.
- From now on: every change goes on a branch + PR; never push to `main` directly.

## Next
- After Day 1 PR merges: Day 2, `internal/compose` (port of `spikes/b/rewrite.py`) + golden tests.

## Open questions
- Licence for WorldTree (MIT / Apache-2.0?) and whether AGPL/GPL compose files may be used as testdata.
- Installing worktrunk for Day 11 end-to-end test needs approval.
- Unverified on macOS/Linux and with real worktrunk.
