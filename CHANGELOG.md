# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

When a new release is proposed:

1. Create a new branch `bump/x.x.x` (this isn't a long-lived branch!!!);
2. The Unreleased section on `CHANGELOG.md` gets a version number and date;
3. Open a Pull Request with the bump version changes targeting the `main` branch;
4. When the Pull Request is merged, a new Git tag must be created using <LINK TO THE PLATFORM TO OPEN THE PULL REQUEST>.

Releases to productive environments should run from a tagged version.
Exceptions are acceptable depending on the circumstances (critical bug fixes that can be cherry-picked, etc.).

## [Unreleased]

## [0.3.2] - 2026-05-09

### Changed

- renamed the git committer identity used by the refresh workflow from `rios0rios0-bot` to `config-bot` so the bot identity reflects this project's scope rather than the org

### Fixed

- fixed the per-repo "PR already exists" check in `.github/workflows/config-and-docs-refresh.yaml` to filter by `--state open`; `gh pr view <branch>` previously matched merged/closed PRs as well, so once a refresh PR was merged the next run logged `prs_updated` and skipped `gh pr create`, leaving the new force-pushed commit stranded on a branch with no open PR (observed on `rios0rios0/guide` where PR #55 was merged and the 2026-05-04 run force-pushed `f3ae5d3` without opening a new PR)
- wrapped the new `gh pr list` call in `.github/workflows/config-and-docs-refresh.yaml` in an `if !` conditional so a transient `gh` failure (auth, network, permissions) is captured as `failed+=("pr-list: ${target_repo}")` and the per-repo cleanup runs, instead of `set -euo pipefail` aborting the whole batch mid-loop on a bare command substitution

## [0.3.1] - 2026-05-08

### Changed

- bumped the Go directive in `go.mod` from `1.26.2` to `1.26.3` and updated the indirect dependency `golang.org/x/sys` from `v0.43.0` to `v0.44.0`

## [0.3.0] - 2026-04-28

### Added

- added `max_turns` `workflow_dispatch` input to `.github/workflows/config-and-docs-refresh.yaml` (default `50`, was hard-coded `30`) so the per-repo `claude --max-turns` cap can be raised at queue time when a legitimately complex repo trips the safety limit; observed in run `25008617829` where `rios0rios0/gitforge` exited with `Error: Reached max turns (30)` after processing the 9 simpler repos in its batch
- added `quota_skipped` tracking and a fail-fast skip path to the per-batch refresh loop: when `claude` fails and its captured output contains `monthly usage limit`, the loop sets a flag that short-circuits every remaining repo (printing them as `(SKIPPED: monthly Claude usage limit hit earlier in batch)` and adding them to a new `quota_skipped` summary line) instead of re-invoking `claude` for ~3 minutes per repo against the same exhausted quota; the first quota-hitting repo is recorded as `claude-quota: <repo>` in `failed` so the overall batch still exits non-zero

### Changed

- raised the default `max_parallel` for the refresh matrix from `1` to `2` so the weekly run finishes in roughly half the wall-clock; the per-batch sequential drip still keeps the steady-state Anthropic request rate well inside the per-minute budget
- refreshed `.github/copilot-instructions.md` to replace the non-existent `ComplianceIssue` entity with the actual types (`SecuritySettings`, `BranchProtection`, `Ruleset`) and added the missing `claude-code-review.yaml` and `claude.yaml` workflows to the tree diagram
- renamed `.github/workflows/ai-docs-refresh.yaml` to `.github/workflows/config-and-docs-refresh.yaml`, `scripts/refresh_ai_docs_prompt.md` to `scripts/refresh_config_and_docs_prompt.md`, the stable PR branch from `chore/ai-docs-refresh` to `chore/config-and-docs-refresh`, and the concurrency group to `config-and-docs-refresh` so the workflow's name reflects its broader scope (configuration + documentation); today the in-scope set is still `CLAUDE.md` and `.github/copilot-instructions.md`, and the rename leaves room for future targets like diagrams or additional config files without renaming again
- updated the per-repo commit message and PR title produced by the refresh workflow from `chore(ai-docs): refreshed AI assistant guidance` to `chore(refresh): refreshed configuration and documentation` to match the broadened scope

### Fixed

- fixed the per-batch refresh loop in `.github/workflows/config-and-docs-refresh.yaml` (formerly `ai-docs-refresh.yaml`) to redirect `</dev/null` into the `claude -p` invocation; without it `claude` inherited the outer `while read` loop's stdin (the `jq` pipe), drained the rest of the batch's repo list after the first iteration, and silently exited with a misleading per-batch summary — observed in run `24982782411` where every batch processed only `[1/N]` repos before reporting success

## [0.2.1] - 2026-04-25

### Changed

- renamed the repository from `fleet-maintenance` to `config-automation` and updated the Go module path from `github.com/rios0rios0/fleet-maintenance` to `github.com/rios0rios0/config-automation`; all internal imports, `README.md`, `CONTRIBUTING.md`, `.github/copilot-instructions.md`, and the `ai-docs-refresh.yaml` workflow were updated in lockstep

## [0.2.0] - 2026-04-24

### Added

- added `.github/workflows/claude-code-review.yaml`, the PR-opened/synchronize/reopen wrapper that calls the reusable `rios0rios0/.github/.github/workflows/claude-code-review.yaml@main` workflow with `secrets: inherit` so every new PR on this repo gets an automated Claude Code review
- added `.github/workflows/claude.yaml`, the issue/PR-comment wrapper that calls the reusable `rios0rios0/.github/.github/workflows/claude.yaml@main` workflow with `secrets: inherit` so `@claude` mentions on issues, PR comments, and PR reviews trigger the Claude Code assistant (gated to `OWNER`/`MEMBER`/`COLLABORATOR` by the reusable workflow)
- added `batch_size` and `max_parallel` `workflow_dispatch` inputs so the matrix shape can be retuned per-run without editing the workflow; defaults preserve the previous serial rate-limit behavior
- added a per-batch summary footer (`no_drift / prs_created / prs_updated / failed`) and a `--max-turns 30` safety cap on each `claude` invocation so a stuck reasoning loop cannot exhaust the job-timeout budget

### Changed

- changed `.github/workflows/ai-docs-refresh.yaml` to a batched-matrix shape: the `discover` job now chunks the sorted `harden-repos --list-json` output into groups of `batch_size` repos (default `10`) and the `refresh` job runs one leg per batch (`max_parallel: 1` by default) that installs `@anthropic-ai/claude-code` via `npm` and loops through its batch sequentially, replacing the former one-job-per-repo matrix that relied on `anthropics/claude-code-action@v1`
- changed the Actions-pins note in `CLAUDE.md` and `.github/copilot-instructions.md` to drop `anthropics/claude-code-action@v1` and add `actions/setup-node@v4` now that the workflow installs the Claude Code CLI directly via `npm`

### Fixed

- changed the drift-detection step in `.github/workflows/ai-docs-refresh.yaml` to build a `drift_paths` list of existing AI-doc files and treat "no AI-doc files present" as no drift, preventing `git diff` from being invoked with a pathspec that doesn't exist in the repo
- changed the per-batch `run` script in `.github/workflows/ai-docs-refresh.yaml` from `set -uo pipefail` to `set -euo pipefail` so a failing `cat`, `jq`, or similar unchecked command aborts the batch instead of emitting misleading per-repo failures
- fixed `GoGithubBranchProtectionsRepository.FindRulesetByName` to translate `403 Upgrade to GitHub Pro` responses into `repositories.ErrRulesetNotFound` so the daily compliance audit no longer errors on every private repo on GitHub Free — mirrors the existing 403/upgrade-required handling in `FindProtectionByBranch` and lets `AuditResult.ComputeIssues` apply its private-repo carve-out
- fixed the `discover` job in `.github/workflows/ai-docs-refresh.yaml` to validate `inputs.batch_size` before passing it to `jq`, falling back to `10` when the value is not a positive integer so a malformed `workflow_dispatch` input can no longer crash the matrix build
- fixed the tool-allowlist description in `CLAUDE.md` to spell out the fully-scoped `Write(/CLAUDE.md),Write(/.github/copilot-instructions.md),Write(/CHANGELOG.md)` entries instead of the `Write(...)` shorthand, matching the actual CLI args passed to `claude -p`

### Removed

- removed the unused `id-token: write` permission from the `refresh` job in `.github/workflows/ai-docs-refresh.yaml` since the workflow authenticates via a PAT and the Claude Code OAuth token rather than OIDC

## [0.1.1] - 2026-04-22

### Changed

- changed the Go version to `1.26.2` and updated all module dependencies
- converted `entities.DesiredRepoSettings` and `entities.DesiredWikiAllowlist` from package-level variables to functions, keeping the compliance policy immutable from call sites
- renamed the `repositories.RepositoriesRepository` port to `repositories.Repository` to remove the package-name stutter flagged by `revive`

### Fixed

- fixed all `golangci-lint` findings surfaced by CI on the `0.1.0` bump PR: `forbidigo` (table output now uses `fmt.Fprintf(os.Stdout, ...)` instead of `fmt.Print*`), `goconst` (extracted `SecurityStateEnabled`/`SecurityStateDisabled`/`SecurityStateUnknown`), `mnd` (named phase constants `phaseAudit`, `phaseApplyRepo`, `phaseApplySecurity`, `phaseApplyProtection`, `phaseReport`, `exitUsageError`, `secretColumnWidth`, `tableWidth`, `githubListPerPage`), `govet` shadow (renamed inner `err` shadows), `nilnil` (replaced `return nil, nil` with a new `repositories.ErrRulesetNotFound` sentinel handled by `AuditRepositoriesCommand`), `gocognit`/`nestif` (extracted per-concern helpers in `ApplyBranchProtectionCommand`, `ApplySecuritySettingsCommand`, `AuditResult.ComputeIssues`, `mapRulesetToEntity`, `printAuditTable`, and `diffAudits`), and `funlen` (split `diffAudits` into `diffRepoSettings`, `diffSecurity`, `diffBranchProtection`, `diffRuleset`)

## [0.1.0] - 2026-04-21

### Added

- added `.github/copilot-instructions.md`, the AI-assistant context file summarizing the project's architecture, Clean Architecture invariants, build/test/lint commands, environment variables, and policy-change workflow so Copilot / Cursor / Claude Code have consistent grounding without reloading the whole codebase
- added `.github/workflows/ai-docs-refresh.yaml`, the weekly matrix workflow that runs `anthropics/claude-code-action@v1` against every non-fork non-archived `rios0rios0` repo to refresh `CLAUDE.md` and `.github/copilot-instructions.md` and opens a drift PR on `chore/ai-docs-refresh` (migrated from `rios0rios0/.github`)
- added `.github/workflows/repo-compliance-audit.yaml`, the daily scheduled workflow that runs the Go `harden-repos` CLI with `--phase 1 --fail-on-noncompliant` and fails CI when any `rios0rios0` repo drifts from the compliance policy (originally migrated from `rios0rios0/.github` as a Python script, then ported to Go)
- added `.golangci.yaml`, `.gitignore`, and `go.mod` (Go 1.26) with the team-standard linter baseline
- added `cmd/harden-repos/`, a Go CLI following Clean Architecture that enforces repo settings, Dependabot, secret scanning, branch protection, and the `main-protection` ruleset across every `rios0rios0` GitHub repository — supports phases 1-5, `--list-json`, `--dry-run`, `--repo` filter, and `--fail-on-noncompliant`
- added `internal/domain/commands/` with one command per phase (`AuditRepositoriesCommand`, `ApplyRepositorySettingsCommand`, `ApplySecuritySettingsCommand`, `ApplyBranchProtectionCommand`, `ListTargetRepositoriesCommand`, `ReportComplianceChangesCommand`) — each command exposes a listeners struct that maps outcomes to the CLI (controller) layer
- added `internal/domain/entities/` covering `Repository`, `SecuritySettings`, `BranchProtection`, `Ruleset`, and `AuditResult`, with `compliance_policy.go` as the single source of truth for every policy constant (`DesiredRepoSettings`, `DesiredWikiAllowlist`, `DesiredReviewCount`, `DesiredRulesetName`, `DesiredDefaultBranch`, `RepositoryAdminActorType`/`ID`)
- added `internal/domain/repositories/` with three small port interfaces (`Repository`, `SecuritySettingsRepository`, `BranchProtectionsRepository`) so the domain layer never imports the `github.com/google/go-github/v66` SDK
- added `internal/infrastructure/repositories/` with three `GoGithub…Repository` adapters wrapping `github.com/google/go-github/v66` + `golang.org/x/oauth2`
- added `Makefile` with `build`, `run`, `test`, `lint`, `sast`, `setup`, and `clean` targets; `sast` delegates to the SAST toolchain in `rios0rios0/pipelines` per `.claude/rules/ci-cd.md`
- added `README.md`, `CLAUDE.md`, `CONTRIBUTING.md`, `LICENSE`, and `.editorconfig` to bootstrap the repository
- added `scripts/refresh_ai_docs_prompt.md`, the prompt consumed by the refresh workflow that instructs Claude Code to cover both AI-assistant guidance files, record any refresh in `CHANGELOG.md`, and make no edits when the existing files are accurate
- added Uber Dig dependency injection across every layer (`internal/domain/commands/container.go`, `internal/domain/entities/container.go` no-op, `internal/infrastructure/repositories/container.go`) orchestrated by `internal/container.go` and invoked from `cmd/harden-repos/dig.go`
- added unit tests for every command under the `//go:build unit` tag using the `_test` external package, `t.Parallel()`, BDD `// given / // when / // then` blocks, and in-memory doubles preferred over mocks per `.claude/rules/testing.md`; `test/domain/builders/` hosts fluent `RepositoryBuilder` and `AuditResultBuilder` factories and `test/domain/doubles/repositories/` hosts the per-port in-memory doubles

### Changed

- changed `ai-docs-refresh.yaml` to self-checkout `rios0rios0/config-automation` and read `scripts/refresh_ai_docs_prompt.md` locally instead of fetching it from `rios0rios0/.github` via `gh api`, removing the last hardcoded cross-repo dependency and one network round-trip per refresh
- expanded the `anthropics/claude-code-action@v1` allowlist in `ai-docs-refresh.yaml` to include `Edit(/CHANGELOG.md)` and `Write(/CHANGELOG.md)` so Claude can record every AI-docs refresh in the target repo's changelog
- switched both workflows from `actions/setup-python@v6` + `python3` to `actions/setup-go@v6` + `go run ./cmd/harden-repos` so the scheduled jobs exercise the same Go binary the team maintains locally
- updated `scripts/refresh_ai_docs_prompt.md` to require Claude to add a short `[Unreleased]` entry to the target repo's `CHANGELOG.md` whenever it edits `CLAUDE.md` or `.github/copilot-instructions.md`, and to skip the entry when the target repo has no changelog
- widened the drift-detection step in `ai-docs-refresh.yaml` to stage `CHANGELOG.md` alongside the AI-docs files while keeping the diff gate scoped to the AI docs, so a stray CHANGELOG-only edit cannot open a spurious PR

### Removed

- removed `scripts/harden_repos.py` (superseded by the Go CLI at `cmd/harden-repos/`). The Go port preserves every carve-out from the Python original: fork exclusion for Dependabot and secret scanning, private-repo skip for `AllowAutoMerge`, `secret_scanning`, branch protection, and the ruleset, `DesiredWikiAllowlist` for legitimate wiki repos, and the tri-state distinction between `dependabot_alerts=unknown` and `dependabot_alerts=off`

