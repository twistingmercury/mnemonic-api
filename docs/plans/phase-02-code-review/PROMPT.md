# Ralph Loop Prompt — Phase 2 Code Review Fixes

You are executing one Ralph loop cycle for the `mnemonic-api` repository.
Follow the repository-specific rules below.

## Objective

Complete exactly one unchecked cycle from the active PRD, verify it, update the
PRD state, append a progress entry, and stop.

## Inputs

You will be given:

- the active PRD
- the current progress log, if one exists
- the repository working tree

The active PRD for this project is:

- `docs/plans/phase-02-code-review/PRD.md`

The primary supporting documents are:

- `docs/plans/phase-02-code-review/PRD.md`
- `docs/code-reviews/phase-02-queue-publishing.md`
- `src/internal/queue/rabbitmq/publisher.go`
- `src/internal/service/pattern/service.go`
- `src/internal/service/pattern/service_test.go`
- `src/internal/server/server.go`
- `src/tests/docker-compose.yaml`

The canonical progress log path for this repository is:

- `docs/plans/phase-02-code-review/progress.txt`

If `docs/plans/phase-02-code-review/progress.txt` does not exist, create it when
completing the first cycle.

## Non-Negotiable Rules

1. Execute exactly one PRD cycle.
2. Work only on the first unchecked `- [ ]` cycle in the PRD.
3. Do not skip ahead.
4. Do not combine multiple cycles into one run.
5. Search the repository before editing. Do not assume code or files are
   missing.
6. Respect the cycle's `Agent`, `Files`, `Steps`, and `Verify` fields.
7. Keep changes scoped to the selected cycle.
8. Run verification before marking the cycle complete.
9. Update the PRD and progress log only after the cycle passes verification.
10. Stop after finishing that one cycle.

## Repo-Specific Build and Test Rules

- Go module root: `src/` — all `go` commands run from there unless otherwise stated.
- Go version: 1.26+.
- Module path: `github.com/twistingmercury/mnemonic-api`.
- Full build: `make build` from the repo root — runs linters, unit tests, Docker image build, and E2E tests. Required for every cycle; a cycle is not done until `make build` exits 0.
- Race detector: `cd src && go test -race ./...` — required for Cycle 1 only, run before `make build`; the Docker build does not run the race detector.
- Signed commits: use `git commit -S`; if signing fails, stop and report — do not use `--no-gpg-sign`.
- User handles push/merge: do not push or merge; commit only.

## Ralph Loop Procedure

### Step 1: Read the PRD and select the cycle

- Open `docs/plans/phase-02-code-review/PRD.md`.
- Find the first unchecked `- [ ]` cycle under `## Implementation Plan`.
- Extract: cycle title, cycle description, `Agent`, `Files`, `Steps`, `Verify`.

If no unchecked cycle exists, stop and report that the PRD is complete.

### Step 2: Read supporting context

- Read every file listed in the cycle's `Files` field before touching anything.
- Read `docs/code-reviews/phase-02-queue-publishing.md` for context on any finding referenced by the cycle.
- Read the progress log if it exists.
- Search the codebase before editing — never assume code is missing.

### Step 3: Plan narrowly

- Form a minimal plan that completes only the selected cycle.
- Do not plan future cycles.
- Do not expand scope beyond the listed files and directly necessary support files.

### Step 4: Delegate or execute

- Delegate to the cycle's named `Agent` using the Agent tool.
- Keep the implementation bounded to the selected cycle.

### Step 5: Verify

Run the cycle's `Verify` command exactly as written.

Baseline check for every cycle:

- `make build` — runs goimports, golangci-lint, govulncheck, gosec, unit tests, Docker image build, and E2E tests in a single command. Must exit 0.

Additional for Cycle 1 only (race detector is not run inside Docker):
- `cd src && go test -race ./...` — run before `make build`; no data races reported.

If any check fails:

- fix the problem if it is within the cycle scope
- rerun verification
- do not mark the cycle complete until all checks pass

### Step 6: Commit and tag the changes

After verification passes, stage and commit all files produced or modified by
the cycle:

- stage only the files listed in the cycle's `Files` field and any directly
  necessary support files the cycle required
- use a concise commit message naming the cycle number and title (e.g. `Cycle 1 - Publisher mutex and ctx-aware reconnect`)
- use `git commit -S` (signed)
- do not skip hooks
- if the commit fails, fix the issue and recommit before proceeding

After the commit succeeds, create a signed annotated tag as a rollback point:

- tag name: `phase-02-cr-cycle-N` where N is the cycle number (e.g. `phase-02-cr-cycle-1`)
- use `git tag -s phase-02-cr-cycle-N -m "Phase 2 CR Cycle N - <title>"`
- if tag signing fails, stop and report — do not create an unsigned tag

### Step 7: Update project records

After the tag succeeds:

- change the selected PRD cycle from `- [ ]` to `- [x]`
- append a concise entry to `docs/plans/phase-02-code-review/progress.txt`
- stage and commit the updated PRD and progress log as a follow-up commit
- tag the follow-up commit with `phase-02-cr-cycle-N-records` (e.g. `phase-02-cr-cycle-1-records`)

Each progress entry should include:

- cycle number and title
- date
- summary of work completed
- verification performed
- important follow-up notes, if any

### Step 8: Report and stop

At the end of the loop:

- report what changed
- report what verification passed
- report the next unchecked cycle
- stop

Do not continue into the next cycle.

## Failure Modes to Avoid

- completing more than one cycle in one run
- editing files unrelated to the selected cycle
- skipping repository search and duplicating existing code
- marking a cycle complete before verification passes
- marking a cycle done without running `make build`
- skipping `go test -race ./...` for Cycle 1
- creating unsigned tags or skipping the tag entirely
- pushing or merging (user handles git remote operations)

## Output Contract

Your final response for a completed loop should contain:

- the completed cycle number and title
- the files changed
- the verification that passed
- the next unchecked cycle

If you could not complete the cycle, state exactly why and do not mark it done.
