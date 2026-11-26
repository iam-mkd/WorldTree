# STATE — WorldTree

## Approved / decided (details in DECISIONS.md)
- Spikes A and B done (Windows 11 + Docker Desktop). Reports: `spikes/a/REPORT.md`, `spikes/b/REPORT.md`.
- Scope narrowed: plug into worktrunk hooks; no worktree manager; no DB cloning/slicing in MVP.
- Rewrite = generated compose override (`!reset`/`!override`) from `docker compose config` JSON.
- Shipping: local CLI, single Go binary, GitHub Releases + Homebrew tap. Name `worldtree` is free on Homebrew.
- Docker images used by spikes removed (19.6 GB -> 7.4 GB).

## Proposed, awaiting approval
- `ARCHITECTURE.md` and `PLAN.md` (14 days, 40 h, Day 1 = Fri 2026-11-27).
- Windows = best-effort CI build + unit tests (assumed, not confirmed).

## Next
- User creates the GitHub repo and pushes this folder; work continues in that repo.
- Day 1: Go scaffold, cobra, Makefile, CI, `worldtree version`.

## Open questions
- Licence for WorldTree (MIT / Apache-2.0?) and whether AGPL/GPL compose files may be used as testdata.
- Installing worktrunk for Day 11 end-to-end test needs approval.
- Unverified on macOS/Linux and with real worktrunk.
