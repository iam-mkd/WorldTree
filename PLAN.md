# WorldTree — 14-day MVP plan

Budget: 10 weekdays x 2 h + 4 weekend days x 5 h = **40 h**, no buffer built in.
Each day ends with a commit, passing tests and a demo command. Day 1 is assumed to be Fri 2026-11-27.

| Day | Date | h | Slice | Demo / done when |
|---|---|---|---|---|
| 1 | Fri 11-27 | 2 | Repo scaffold: Go module, cobra, Makefile, CI (lint + unit on ubuntu/macOS/windows), `testdata/` from spikes | `worldtree version`; CI green |
| 2 | Sat 11-28 | 5 | `internal/compose`: config JSON → analysis → override render (port of `spikes/b/rewrite.py`), findings for unsupported features; golden tests on immich/mastodon/paperless/edge | `worldtree render testdata/compose/mastodon` |
| 3 | Sun 11-29 | 5 | `internal/registry` (JSON + flock, atomic write) + `internal/ports` (range allocator, 4-address probe); multi-process concurrency test | 20 parallel allocations, zero duplicates; held port skipped |
| 4 | Mon 11-30 | 2 | `up` v1: register intent → lease → render → `compose up -d --wait` → mark ready | `worldtree up` on the Spring sample, twice in two dirs |
| 5 | Tue 12-01 | 2 | `down` + `ps` | `worldtree ps` shows 2 worlds; `down` leaves zero labelled resources |
| 6 | Wed 12-02 | 2 | `envfile`: per-service ports, DATABASE_URL, Spring datasource + Redis vars; `worldtree env` | Spring app run on the host against world B |
| 7 | Thu 12-03 | 2 | Integration harness (real Docker, build tag) + ubuntu CI job: two worlds of the sample app are isolated | `make integration` green locally and in CI |
| 8 | Fri 12-04 | 2 | Reconcile: re-running `up` after a failure at each step converges; labels on all resources | test kills `up` at each step, re-runs, gets a ready world |
| 9 | Sat 12-05 | 5 | `gc`: registry x labels x filesystem join, dry-run by default, `--yes`; kill -9 and `rm -rf` integration tests | after kill -9 mid-`up` + `gc --yes`: zero labelled resources, lease freed |
| 10 | Sun 12-06 | 5 | `doctor --prove`: ports, Postgres sentinel (pgx), Redis sentinel (RESP), volume/bind sharing check; `--json` | `worldtree doctor --prove` passes on 2 worlds, fails on a deliberately shared volume |
| 11 | Mon 12-07 | 2 | `worldtree init` writes worktrunk `pre-start`/`pre-remove` hooks; end-to-end with real worktrunk (install needs your OK) | `wt switch -c feat-x` gives a ready world; `wt remove` tears it down |
| 12 | Tue 12-08 | 2 | Unsupported-feature UX (host network, fixed subnet, external volume): clear errors + docs; stretch: fixed-subnet remap | `worldtree up` on edge fixture explains each refusal |
| 13 | Wed 12-09 | 2 | Release: goreleaser, Homebrew tap, checksums; `go install` path; Windows build artifact | `goreleaser release --snapshot` produces all targets |
| 14 | Thu 12-10 | 2 | README (quickstart, why-not-worktrunk/binky table, security section), 2-agent demo script, tag v0.1.0 | fresh-machine quickstart works from README alone |

## Definition of done (MVP)
- Two agents on the Spring sample (and on paperless) run concurrently with no shared port, container, network,
  volume, DB or cache; `doctor --prove` shows it.
- kill -9 at any point of `up`, or deleting a world dir, is fully recovered by `gc`.
- Installable via `brew install <tap>/worldtree` on macOS; tarball on Linux.

## Cut order if behind (first to go)
1. Fixed-subnet remap (day 12 stretch)  2. Windows build artifact  3. `--json` output
4. `ps` health column  5. worktrunk `init` (document the hook lines in README instead).
Never cut: `doctor --prove`, crash-safe reconcile + `gc`, the real-Docker integration test.

## Risks
- 40 h with no buffer; days 9–10 are the riskiest.
- macOS integration only runs by hand (no Docker on hosted macOS runners).
- Compose >= 2.24 required; older installs get a clear error, not a fallback.
- Apps that bake ports/URLs into their own config are out of reach of compose rewriting (documented, not solved).
