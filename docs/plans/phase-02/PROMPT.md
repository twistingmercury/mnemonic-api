# Ralph Loop Prompt — Phase 2: Queue Publishing

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

- `docs/plans/phase-02/PRD.md`

The primary supporting documents are:

- `docs/plans/phase-02/PRD.md`
- `src/internal/config/config.go`
- `src/internal/config/defaults.go`
- `src/internal/server/server.go`
- `src/internal/service/pattern/service.go`
- `src/internal/queue/rabbitmq/subscriber.go` in `mnemonic-enricher` (reference for reconnect pattern — read-only)
- `docker-compose.yaml`
- `src/tests/docker-compose.yaml`

The canonical progress log path for this repository is:

- `docs/plans/phase-02/progress.txt`

If `docs/plans/phase-02/progress.txt` does not exist, create it when completing
the first cycle.

## Non-Negotiable Rules

1. Execute exactly one PRD cycle.
2. Work only on the first unchecked `- [ ]` cycle in the PRD.
3. Do not skip ahead.
4. Do not combine multiple cycles into one run.
5. Search the repository before editing. Do not assume code or files are missing.
6. Respect the cycle's `Agent`, `Files`, `Steps`, and `Verify` fields.
7. Keep changes scoped to the selected cycle.
8. Run verification before marking the cycle complete.
9. Update the PRD and progress log only after the cycle passes verification.
10. Stop after finishing that one cycle.

## Repo-Specific Build and Test Rules

- Go module root: `src/` — all `go` commands run from there unless otherwise stated.
- E2E tests live in a separate module: `src/tests/e2e/` — use `cd src/tests/e2e && go build ./...` to verify E2E compilation.
- Go version: 1.26+.
- Module path: `github.com/twistingmercury/mnemonic-api`.
- Build verification: `go build ./...` must exit 0 before any test run.
- Test command: `go test ./...` from `src/`.
- Analysis: `make analyze` from the repo root runs `goimports`, `golangci-lint run`, `govulncheck`, and `gosec` — run this after any `.go` file is added or modified.
- Shell scripts: validate with `shellcheck` and `bash -n` before marking complete.
- Full build: `make build` from the repo root builds the Docker image and runs E2E tests.
- Signed commits: use `git commit -S`; if signing fails, stop and report — do not use `--no-gpg-sign`.
- User handles push/merge: do not push or merge; commit only.

## Ralph Loop Procedure

### Step 1: Read the PRD and select the cycle

- Open `docs/plans/phase-02/PRD.md`.
- Find the first unchecked `- [ ]` cycle under `## Implementation Plan`.
- Extract: cycle title, description, `Agent`, `Files`, `Steps`, `Verify`.

If no unchecked cycle exists, stop and report that the PRD is complete.

### Step 2: Read supporting context

- Read every file listed in the cycle's `Files` field before touching anything.
- Read `src/internal/server/server.go` for any cycle touching config or wiring.
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

Go baseline checks (run for any cycle that produces or modifies `.go` files):

1. `cd src && go build ./...` — must exit 0
2. `cd src && go vet ./...` — no vet errors
3. `cd src && go test ./...` — all tests pass
4. `make analyze` — goimports, golangci-lint, govulncheck, gosec all pass

**Important scope notes:**
- Cycle 3 verify is scoped to `./internal/service/pattern/...` — this is intentional because Cycle 4 updates the server call site. Do not widen the scope.
- Cycle 5 verify uses `cd src/tests/e2e && go build ./...` — E2E tests live in a separate module.

If any check fails:

- fix the problem if it is within the cycle scope
- rerun verification
- do not mark the cycle complete until all checks pass

### Step 6: Commit the changes

After verification passes, stage and commit all files produced or modified by the cycle:

- stage only the files listed in the cycle's `Files` field and any directly necessary support files
- use a concise commit message naming the cycle number and title (e.g. `Cycle 2 - Publisher package`)
- use `git commit -S` (signed)
- do not skip hooks
- if the commit fails, fix the issue and recommit before proceeding

### Step 7: Update project records

After the commit succeeds:

- change the selected PRD cycle from `- [ ]` to `- [x]`
- append a concise entry to `docs/plans/phase-02/progress.txt`
- stage and commit the updated PRD and progress log as a follow-up commit

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
- widening Cycle 3 verify beyond `./internal/service/pattern/...`
- skipping `make analyze` after modifying any `.go` file
- leaving `internal/enricher`, `internal/service/enrichment`, or extraction files in the repo after Cycle 4
- pushing or merging (user handles git remote operations)

## Output Contract

Your final response for a completed loop should contain:

- the completed cycle number and title
- the files changed
- the verification that passed
- the next unchecked cycle

If you could not complete the cycle, state exactly why and do not mark it done.
