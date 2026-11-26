# DECISIONS

## 2026-11-26 — Narrow scope after Spike A (user approved "Narrow, run Spike B")
- Don't build a worktree manager; integrate with worktrunk hooks (`pre-start`, `pre-remove`) instead.
- Defer per-world DB cloning / Postgres slicing: compose-per-world already gives each world its own Postgres.
- Why: worktrunk-style hook + 25-line override covered ~85-90% of the pain in Spike A.

## 2026-11-26 — Rewrite via generated override, not by rewriting the file (Spike B)
- Normalise with `docker compose config --format json`, emit override with `!reset` / `!override`.
- Why: original stays authoritative (relative build contexts, env_file keep working); compose itself resolves
  include/extends/anchors/profiles; resolved secrets are only read in memory, never written.
- Cost: needs Compose >= 2.24.

## 2026-11-26 — Shipping: local CLI, single Go binary (user approved)
- GitHub Releases + Homebrew tap (+ `go install`); no daemon, server, GUI or Docker image.
- Why: everything reconciles from registry + Docker labels; nothing long-running to drift. Tool needs host
  git worktrees and port probing, so a container image doesn't fit; worktrunk has hooks, not plugins.
- Name `worldtree`: no Homebrew formula/cask; 119 GitHub repos contain the word, none a dev CLI. Binary is
  never `wt` (worktrunk's).

## 2026-11-26 — Architecture choices that differ from the brief (see ARCHITECTURE.md)
- Docker CLI (`--format json`), not the Engine API: compose has no Engine API; CLI honours Docker contexts.
- JSON + flock registry, not SQLite: tiny data, no cgo, hand-repairable.
- Windows: best-effort (build + unit tests in CI). ASSUMED — user develops on Windows but did not confirm.
- Third-party compose files (AGPL/GPL) not committed; licence check before using them as testdata.
