# Code Review: Push Readiness for v0.3.1

> **Version**: v01
> **Date**: 2026-08-19
> **Notes**: Review of commit `258b2bd` against `3296281`, updated with local release-hardening fixes and the accepted single-workflow CI design before pushing branch `feature/update` and tag `v0.3.1`.

**Reviewers:** code_reviewer, solutions_architect, shell_script_engineer
**Phase:** Push readiness (v0.3.1)

## Files Reviewed

### Source Files

- `.dockerignore` - Root Docker build-context allowlist.
- `.github/workflows/mnemonic-ci.yaml` - Docker-first build, test, and publication workflow.
- `.github/workflows/mnemonic-cd.yaml` - Deleted separate publication workflow.
- `.gitignore` - Local Go binary exclusion.
- `CHANGELOG.md` - Release history.
- `README.md` - Project onboarding and runtime documentation.
- `docker-compose.yaml` - Local multi-service stack.
- `src/.dockerignore` - Application-context exclusions.
- `src/.gitignore copy` - Deleted duplicate ignore file.
- `src/build/Dockerfile` - Application image build and runtime definition.
- `src/build/build.sh` - Build, test, and E2E entrypoint.
- `src/go.mod` and `src/go.sum` - Go version and dependency upgrades.
- `src/loader` - Deleted tracked binary.

### Test Files

- `src/tests/e2e/go.mod` - E2E Go toolchain version.
- `src/tests/docker-compose.yaml` - E2E image selection and orchestration, reviewed as part of the build chain.

## Validation Results

| Tool | Result |
| ---- | ------ |
| `make build` | PASS: image build, lint, vulnerability scan, security scan, unit tests, REST E2E, and MCP E2E; OpenAI-dependent search tests skipped without a configured key |
| `shellcheck src/build/build.sh` | PASS |
| `bash -n src/build/build.sh` | PASS |
| CI workflow YAML parsing | PASS |
| `docker compose -f src/tests/docker-compose.yaml config --quiet` | PASS |
| `docker compose -f docker-compose.yaml config --quiet` | FAIL: `dev_enricher` depends on undefined service `migrate` |
| `markdownlint README.md` | PASS |
| `git diff --check 3296281...258b2bd` | FAIL: trailing whitespace in `docker-compose.yaml` |
| Worktree status | Clean at review start; commit `258b2bd` is locally tagged `v0.3.1` |

### Post-Fix Revalidation

| Tool | Result |
| ---- | ------ |
| `docker compose -f docker-compose.yaml config --quiet` | PASS with a validation-only OpenAI key |
| `docker compose -f src/tests/docker-compose.yaml config --quiet` | PASS with an explicit validation image reference |
| CI workflow YAML parsing | PASS |
| `markdownlint README.md CHANGELOG.md docs/code_reviews/push_readiness_v01.md` | PASS |
| `git diff --check` | PASS |
| README local-link checks | PASS |

## Design Compliance

The accepted Docker-first design uses one workflow. Every run builds and tests first; only successful pushes to `main` or `develop` execute the registry login and publication steps. Those steps push the tested SHA image, promote `latest` or `latest-dev`, and publish an exact Semantic Version tag when one points at the `main` commit. The local stack, release metadata, supported entrypoint validation, exact E2E image selection, and pre-migrated database test contract are aligned.

### Behavioral Requirements Verified

- Docker image quality gates and unit tests run before compilation. ✓
- REST and MCP E2E suites complete through `make build`. ✓
- Pull-request runs cannot execute the registry login or image publication steps. ✓
- Main and develop publications reuse the image built and tested earlier in the same job. ✓
- A valid Semantic Version tag on the main commit is published without rebuilding. ✓
- Runtime image uses a non-root user and includes Apache-2.0 license metadata and text. ✓

### Design Doc Divergences (Post-Review)

Post-review fixes aligned the implementation and release documentation with the Docker-first design.

#### Naming Divergences

| Old Name (in design docs) | New Name (in implementation) | Reason |
| ------------------------- | ---------------------------- | ------ |
| None | None | No naming divergences identified |

#### Structural Divergences (justified improvements over design doc)

| Divergence | Design Doc | Implementation | Assessment |
| ---------- | ---------- | -------------- | ---------- |
| Single CI workflow | Docker-first build-before-publish requirement | One job builds and tests, then conditionally publishes on main or develop pushes | Accepted simpler design aligned with the companion mnemonic-mcp project |

#### Documents Updated

| Document | Scope | Status |
| -------- | ----- | ------ |
| `docs/code_reviews/push_readiness_v01.md` | Push-readiness evidence, fixes, and revalidation | Updated in place |
| `README.md` | Release version and local workflow | Updated |
| `CHANGELOG.md` | v0.3.1 release summary and Swagger terminology | Updated |

## Findings

### HIGH Priority

| ID | Source | Finding | Resolution |
| -- | ------ | ------- | ---------- |
| H1 | All reviewers | `docker-compose.yaml` is invalid because `dev_enricher` depends on undefined `migrate`. The referenced `mnemonic-mcp` stack uses the same pre-migrated `mnemonic-postgres:v1.0.0-dev` image and defines no migration service. | **Resolved:** removed the undefined dependency; root Compose configuration now validates. |
| H2 | code_reviewer, solutions_architect | RabbitMQ creates `mnemonic/mnemonic_dev`, while `dev_enricher` uses `guest/guest`; enrichment cannot authenticate consistently. | **Resolved:** the broker, API, and enricher now use `mnemonic/mnemonic_dev`. |
| H3 | code_reviewer, solutions_architect | Commit `258b2bd` is tagged `v0.3.1`, but README and changelog declare `v0.2.1`; the tag is not on origin. | **Resolved:** README and changelog now declare v0.3.1 while preserving v0.2.1 history. |

### MEDIUM Priority

| ID | Source | Finding | Resolution |
| -- | ------ | ------- | ---------- |
| M1 | All reviewers | CI path filters omit root `docker-compose.yaml` and `Makefile`, and CI never validates the root stack. | **Resolved:** CI watches both files and validates the root Compose configuration. |
| M2 | shell_script_engineer, solutions_architect | CI/CD no longer publishes SemVer image tags, and tag pushes do not trigger CI. | **Resolved by release policy:** a successful main push publishes a Docker-compatible exact Semantic Version when that tag points at the built commit; standalone tag pushes are not part of the accepted workflow. |
| M3 | code_reviewer | Concurrent successful CD runs can finish out of order and move `latest` or `latest-dev` backward. | **Accepted:** the separate CD workflow was removed; branch promotion occurs in the successful CI job, with out-of-order concurrent completion retained as a known tradeoff of the simpler design. |
| M4 | Reviewer disagreement | E2E Compose references `:latest`, while CI exports the SHA alias. Current Compose v5.5.0 empirically uses the local alias, but documented/default behavior may pull `latest`; exact tested-artifact provenance is not structurally enforced. | **Resolved:** E2E Compose consumes the exported `MNEMONIC_API_IMAGE`; the build invokes Compose with `--pull never` and verifies the tested container image ID. |
| M5 | solutions_architect | `make start` suppresses Compose failures with `\|\| true`, so it can report success for an invalid stack. | **Resolved:** Compose startup failures now propagate. |
| M6 | code_reviewer | Runtime database, MCP, queue, vector, and telemetry dependencies were broadly upgraded; `make tests-db` was not recorded in this review. | **Resolved by test policy:** obsolete standalone database targets were removed; `make build` uses pre-migrated database images and remains the full REST and MCP E2E integration gate. |

### LOW Priority

| ID | Source | Finding | Resolution |
| -- | ------ | ------- | ---------- |
| L1 | code_reviewer, solutions_architect | README says `dev_mcp` uses `pull_policy: never`, but that policy was removed. | **Resolved:** README now documents service-level `pull_policy: never` only for `dev_api`; `dev_mcp` and `dev_enricher` use normal registry behavior. |
| L2 | solutions_architect | Changelog says OpenAPI 3.0 while the build generates Swagger 2.0. | **Resolved:** changelog terminology now identifies Swagger 2.0. |
| L3 | shell_script_engineer | Docker pins `swag` v1.16.6, while `make docs-swagger` installs `@latest`. | **Resolved:** the Makefile target now installs v1.16.6. |
| L4 | All reviewers | `docker-compose.yaml` contains trailing whitespace. | **Resolved:** trailing whitespace was removed and `git diff --check` passes. |
| L5 | solutions_architect | The commit subject understates a dependency sweep, Docker hardening, CI redesign, Compose topology change, docs update, and artifact cleanup. | **Advisory:** use a release-oriented commit message when committing the post-review fixes. |
| L6 | solutions_architect | The builder image and GitHub Actions references remain mutable. | **Accepted:** builder risk remains accepted per user direction; consider automated, reviewed pin updates later. |

## Patterns to Document

Patterns identified that should be added to the project's guidance for installed subagents.

1. Validate every supported Compose file in CI whenever it or the root Makefile changes.
2. Pass the immutable image reference from build to E2E and from E2E to publication.
3. Synchronize release tags, README version, changelog section, and container tag policy before publishing.

## Notes for Future Phases

**Phase v0.3.x** (release hardening): Consider deterministic dependency-update automation.
