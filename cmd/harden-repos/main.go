package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	logger "github.com/sirupsen/logrus"

	"github.com/rios0rios0/config-automation/internal/domain/commands"
	"github.com/rios0rios0/config-automation/internal/domain/entities"
)

const (
	phaseAudit           = 1
	phaseApplyRepo       = 2
	phaseApplySecurity   = 3
	phaseApplyProtection = 4
	phaseReport          = 5
	exitUsageError       = 2
	secretColumnWidth    = 7
	tableWidth           = 155
)

// Logrus structured-logging field keys reused across phases.
const (
	fieldRepo    = "repo"
	fieldApplied = "applied"
	fieldAction  = "action"
	fieldPhase   = "phase"
)

func main() {
	var (
		phase              int
		repoFilter         string
		listJSON           bool
		dryRun             bool
		failOnNonCompliant bool
	)

	flag.IntVar(
		&phase,
		"phase",
		0,
		"audit/apply phase (1-5); 0 means 'all audit phases when --dry-run, otherwise required'",
	)
	flag.StringVar(&repoFilter, "repo", "", "target a single repository by name")
	flag.BoolVar(
		&listJSON,
		"list-json",
		false,
		"emit a JSON array of non-fork non-archived repos for the config-and-docs refresh matrix",
	)
	flag.BoolVar(&dryRun, "dry-run", false, "run phases 1-4 without mutating anything")
	flag.BoolVar(
		&failOnNonCompliant,
		"fail-on-noncompliant",
		false,
		"exit 1 when phase 1 detects any non-compliant repo",
	)
	flag.Parse()

	owner := os.Getenv("HARDEN_OWNER")
	if owner == "" {
		owner = "rios0rios0"
	}

	set := injectCommands()
	ctx := context.Background()

	switch {
	case listJSON:
		runListJSON(ctx, set, owner)
	case dryRun:
		runDryRun(ctx, set, owner, repoFilter)
	case phase == phaseAudit:
		runPhase1(ctx, set, owner, repoFilter, failOnNonCompliant)
	case phase == phaseApplyRepo:
		runPhase2(ctx, set, owner, repoFilter)
	case phase == phaseApplySecurity:
		runPhase3(ctx, set, owner, repoFilter)
	case phase == phaseApplyProtection:
		runPhase4(ctx, set, owner, repoFilter)
	case phase == phaseReport:
		runPhase5(ctx, set, owner)
	default:
		logger.Error("must specify --phase 1..5, --list-json, or --dry-run")
		flag.Usage()
		os.Exit(exitUsageError)
	}
}

// runListJSON emits the JSON array consumed by the config-and-docs refresh matrix.
func runListJSON(ctx context.Context, set commandSet, owner string) {
	var result []entities.Repository
	set.ListTargets.Execute(
		ctx,
		commands.ListTargetRepositoriesInput{Owner: owner},
		commands.ListTargetRepositoriesListeners{
			OnSuccess: func(repos []entities.Repository) {
				result = repos
			},
			OnError: func(err error) {
				logger.WithError(err).Fatal("listing target repos")
			},
		},
	)

	payload := make([]map[string]string, 0, len(result))
	for _, r := range result {
		payload = append(payload, map[string]string{
			"name":           r.Name,
			"default_branch": r.DefaultBranch,
		})
	}

	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(payload); err != nil {
		logger.WithError(err).Fatal("encoding JSON")
	}
}

// runPhase1 audits every repo, prints the table, and writes the before
// snapshot. When --fail-on-noncompliant is set, non-zero exit on any
// issue.
func runPhase1(ctx context.Context, set commandSet, owner, repoFilter string, failOnNonCompliant bool) {
	audits := executeAudit(ctx, set, owner, repoFilter)
	printAuditTable(audits)
	saveSnapshot(audits, auditBeforePath())

	nonCompliant := countNonCompliant(audits)
	if failOnNonCompliant && nonCompliant > 0 {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintf(os.Stderr, "Non-compliant repos: %d/%d\n", nonCompliant, len(audits))
		os.Exit(1)
	}
}

func runPhase2(ctx context.Context, set commandSet, owner, repoFilter string) {
	audits := executeAudit(ctx, set, owner, repoFilter)
	saveSnapshot(audits, auditBeforePath())

	set.ApplyRepo.Execute(ctx, commands.ApplyRepositorySettingsInput{
		Owner:  owner,
		Audits: audits,
	}, commands.ApplyRepositorySettingsListeners{
		OnChange: func(change commands.ApplyRepositorySettingsChange) {
			logger.WithFields(logger.Fields{
				fieldRepo:    change.RepositoryName,
				fieldApplied: change.Applied,
				"new_wiki":   change.NewSettings.HasWiki,
			}).Info("applied repo settings")
		},
		OnSuccess: func(changed, compliant int) {
			logger.WithFields(logger.Fields{"changed": changed, "compliant": compliant}).Info("phase 2 complete")
		},
		OnError: func(name string, err error) {
			logger.WithError(err).WithField(fieldRepo, name).Error("phase 2 error")
		},
	})
}

// runPhase3 mirrors runPhase4's shape but dispatches a different
// command with a distinct listener type, so the duplication is intrinsic.
//
//nolint:dupl // distinct listener/input types prevent a generic extraction
func runPhase3(ctx context.Context, set commandSet, owner, repoFilter string) {
	audits := executeAudit(ctx, set, owner, repoFilter)
	saveSnapshot(audits, auditBeforePath())

	set.ApplySecurity.Execute(ctx, commands.ApplySecuritySettingsInput{
		Owner:  owner,
		Audits: audits,
	}, commands.ApplySecuritySettingsListeners{
		OnChange: func(change commands.ApplySecuritySettingsChange) {
			logger.WithFields(logger.Fields{
				fieldRepo:    change.RepositoryName,
				fieldAction:  change.Action,
				fieldApplied: change.Applied,
			}).Info("applied security setting")
		},
		OnSkip: func(name, reason string) {
			logger.WithFields(logger.Fields{fieldRepo: name, "reason": reason}).Info("skipped")
		},
		OnSuccess: func(secretScanning, dependabot int) {
			logger.WithFields(logger.Fields{"secret_scanning": secretScanning, "dependabot": dependabot}).
				Info("phase 3 complete")
		},
		OnError: func(name string, err error) {
			logger.WithError(err).WithField(fieldRepo, name).Error("phase 3 error")
		},
	})
}

// runPhase4 mirrors runPhase3's shape but dispatches a different
// command with a distinct listener type, so the duplication is intrinsic.
//
//nolint:dupl // distinct listener/input types prevent a generic extraction
func runPhase4(ctx context.Context, set commandSet, owner, repoFilter string) {
	audits := executeAudit(ctx, set, owner, repoFilter)
	saveSnapshot(audits, auditBeforePath())

	set.ApplyProtection.Execute(ctx, commands.ApplyBranchProtectionInput{
		Owner:  owner,
		Audits: audits,
	}, commands.ApplyBranchProtectionListeners{
		OnChange: func(change commands.ApplyBranchProtectionChange) {
			logger.WithFields(logger.Fields{
				fieldRepo:    change.RepositoryName,
				fieldAction:  change.Action,
				fieldApplied: change.Applied,
			}).Info("applied branch protection")
		},
		OnSkip: func(name, reason string) {
			logger.WithFields(logger.Fields{fieldRepo: name, "reason": reason}).Info("skipped")
		},
		OnSuccess: func(changed, skipped int) {
			logger.WithFields(logger.Fields{"changed": changed, "skipped": skipped}).Info("phase 4 complete")
		},
		OnError: func(name string, err error) {
			logger.WithError(err).WithField(fieldRepo, name).Error("phase 4 error")
		},
	})
}

func runPhase5(ctx context.Context, set commandSet, owner string) {
	before, err := loadSnapshot(auditBeforePath())
	if err != nil {
		logger.WithError(err).Fatal("loading before snapshot; run --phase 1 first")
	}
	after := executeAudit(ctx, set, owner, "")
	saveSnapshot(after, auditAfterPath())

	set.Report.Execute(commands.ReportComplianceChangesInput{
		Before: before,
		After:  after,
	}, commands.ReportComplianceChangesListeners{
		OnSuccess: func(diffs []commands.ComplianceDiff, reposChanged int) {
			for _, d := range diffs {
				logger.WithFields(logger.Fields{
					fieldRepo: d.Repository,
					"field":   d.Field,
					"before":  d.Before,
					"after":   d.After,
				}).Info("changed")
			}
			logger.WithField("repos_changed", reposChanged).Info("phase 5 complete")
		},
	})
}

func runDryRun(ctx context.Context, set commandSet, owner, repoFilter string) {
	audits := executeAudit(ctx, set, owner, repoFilter)
	saveSnapshot(audits, auditBeforePath())

	set.ApplyRepo.Execute(ctx, commands.ApplyRepositorySettingsInput{
		Owner:  owner,
		Audits: audits,
		DryRun: true,
	}, commands.ApplyRepositorySettingsListeners{
		OnChange: func(change commands.ApplyRepositorySettingsChange) {
			logger.WithFields(logger.Fields{
				fieldRepo:   change.RepositoryName,
				fieldPhase:  phaseApplyRepo,
				fieldAction: "repo_settings",
			}).Info("would apply")
		},
		OnSuccess: func(_, _ int) {},
		OnError:   func(_ string, _ error) {},
	})

	set.ApplySecurity.Execute(ctx, commands.ApplySecuritySettingsInput{
		Owner:  owner,
		Audits: audits,
		DryRun: true,
	}, commands.ApplySecuritySettingsListeners{
		OnChange: func(change commands.ApplySecuritySettingsChange) {
			logger.WithFields(logger.Fields{
				fieldRepo:   change.RepositoryName,
				fieldPhase:  phaseApplySecurity,
				fieldAction: change.Action,
			}).Info("would apply")
		},
		OnSuccess: func(_, _ int) {},
		OnError:   func(_ string, _ error) {},
	})

	set.ApplyProtection.Execute(ctx, commands.ApplyBranchProtectionInput{
		Owner:  owner,
		Audits: audits,
		DryRun: true,
	}, commands.ApplyBranchProtectionListeners{
		OnChange: func(change commands.ApplyBranchProtectionChange) {
			logger.WithFields(logger.Fields{
				fieldRepo:   change.RepositoryName,
				fieldPhase:  phaseApplyProtection,
				fieldAction: change.Action,
			}).Info("would apply")
		},
		OnSuccess: func(_, _ int) {},
		OnError:   func(_ string, _ error) {},
	})
}

func executeAudit(ctx context.Context, set commandSet, owner, repoFilter string) []entities.AuditResult {
	var out []entities.AuditResult
	set.Audit.Execute(ctx, commands.AuditRepositoriesInput{
		Owner:      owner,
		RepoFilter: repoFilter,
	}, commands.AuditRepositoriesListeners{
		OnProgress: func(i, total int, name string) {
			fmt.Fprintf(os.Stderr, "\r  Auditing %d/%d: %-40s", i+1, total, name)
		},
		OnSuccess: func(audits []entities.AuditResult) {
			out = audits
		},
		OnError: func(err error) {
			logger.WithError(err).Fatal("auditing")
		},
	})
	fmt.Fprintln(os.Stderr)
	return out
}

func printAuditTable(audits []entities.AuditResult) {
	sort.Slice(audits, func(i, j int) bool { return audits[i].Repository.Name < audits[j].Repository.Name })

	printTableHeader()
	nonCompliant := printTableRows(audits)
	printSummary(audits)
	printNonComplianceReport(audits, nonCompliant)
}

func printTableHeader() {
	fmt.Fprintf(
		os.Stdout,
		"\n%-40s %-8s %-7s %-7s %-5s %-5s %-7s %-7s %-7s %-7s %-5s %-6s %-6s %-5s\n",
		"REPO",
		"VIS",
		"DEL-BR",
		"AUTO-M",
		"WIKI",
		"PROJ",
		"SEC-SC",
		"PUSH-P",
		"DEP-AL",
		"DEP-UP",
		"PROT",
		"NO-FP",
		"STALE",
		"SIGS",
	)
	fmt.Fprintln(os.Stdout, stringOfChar('-', tableWidth))
}

func printTableRows(audits []entities.AuditResult) int {
	nonCompliant := 0
	for _, a := range audits {
		repo := a.Repository
		if a.AuditError != "" {
			fmt.Fprintf(os.Stdout, "%-40s ERROR: %s\n", repo.Name, a.AuditError)
			continue
		}

		fmt.Fprintf(os.Stdout, "%-40s %-8s %-7s %-7s %-5s %-5s %-7s %-7s %-7s %-7s %-5s %-6s %-6s %-5s\n",
			repo.Name,
			repo.Visibility,
			yesNo(repo.Settings.DeleteBranchOnMerge),
			yesNo(repo.Settings.AllowAutoMerge),
			yesNo(repo.Settings.HasWiki),
			yesNo(repo.Settings.HasProjects),
			truncate(defaultIfEmpty(a.Security.SecretScanning, "N/A"), secretColumnWidth),
			truncate(defaultIfEmpty(a.Security.PushProtection, "N/A"), secretColumnWidth),
			yesNoTri(a.Security.DependabotAlerts),
			yesNo(a.Security.DependabotUpdates),
			protectionLabel(a),
			forcePushLabel(a),
			yesNo(a.BranchProtection.DismissStaleReviews),
			yesNoTri(a.BranchProtection.Signatures),
		)

		if len(a.ComputeIssues()) > 0 {
			nonCompliant++
		}
	}
	return nonCompliant
}

func protectionLabel(a entities.AuditResult) string {
	switch {
	case a.BranchProtection.Enabled:
		return "Y"
	case !a.BranchProtection.Available:
		return "N/A"
	default:
		return "N"
	}
}

func forcePushLabel(a entities.AuditResult) string {
	if a.HasForcePushRuleset() {
		return "Y"
	}
	return "N"
}

func printSummary(audits []entities.AuditResult) {
	total := len(audits)
	public := 0
	private := 0
	forks := 0
	protected := 0
	unavailable := 0
	for _, a := range audits {
		if a.Repository.Private {
			private++
		} else {
			public++
		}
		if a.Repository.Fork {
			forks++
		}
		if a.BranchProtection.Enabled {
			protected++
		}
		if !a.BranchProtection.Available {
			unavailable++
		}
	}
	fmt.Fprintf(os.Stdout, "\nSummary: %d repos (%d public, %d private, %d forks)\n", total, public, private, forks)
	fmt.Fprintf(os.Stdout, "Branch protection: %d enabled, %d unavailable\n", protected, unavailable)
}

func printNonComplianceReport(audits []entities.AuditResult, nonCompliant int) {
	fmt.Fprintln(os.Stdout, "\n=== NON-COMPLIANCE REPORT ===")
	if nonCompliant == 0 {
		fmt.Fprintln(os.Stdout, "\nAll repos are compliant.")
		return
	}
	for _, a := range audits {
		issues := a.ComputeIssues()
		if len(issues) == 0 {
			continue
		}
		fmt.Fprintf(os.Stdout, "\n  %s (%d):\n", a.Repository.Name, len(issues))
		for _, issue := range issues {
			fmt.Fprintf(os.Stdout, "    - %s\n", issue)
		}
	}
	fmt.Fprintf(os.Stdout, "\nTotal non-compliant: %d/%d\n", nonCompliant, len(audits))
}

func countNonCompliant(audits []entities.AuditResult) int {
	n := 0
	for _, a := range audits {
		if len(a.ComputeIssues()) > 0 {
			n++
		}
	}
	return n
}

func auditBeforePath() string {
	return filepath.Join(os.TempDir(), "gh_hardening_audit_before.json")
}

func auditAfterPath() string {
	return filepath.Join(os.TempDir(), "gh_hardening_audit_after.json")
}

func saveSnapshot(audits []entities.AuditResult, path string) {
	f, err := os.Create(path)
	if err != nil {
		logger.WithError(err).WithField("path", path).Warn("saving snapshot")
		return
	}
	defer func() { _ = f.Close() }()

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	//nolint:musttag // AuditResult is a framework-agnostic entity; json tags belong on infrastructure DTOs only
	if encodeErr := encoder.Encode(audits); encodeErr != nil {
		logger.WithError(encodeErr).WithField("path", path).Warn("encoding snapshot")
		return
	}
	logger.WithField("path", path).Info("audit saved")
}

func loadSnapshot(path string) ([]entities.AuditResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var audits []entities.AuditResult
	//nolint:musttag // AuditResult is a framework-agnostic entity; json tags belong on infrastructure DTOs only
	if decodeErr := json.NewDecoder(f).Decode(&audits); decodeErr != nil {
		return nil, decodeErr
	}
	return audits, nil
}

func yesNo(b bool) string {
	if b {
		return "Y"
	}
	return "N"
}

func yesNoTri(b *bool) string {
	if b == nil {
		return "-"
	}
	return yesNo(*b)
}

func defaultIfEmpty(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func stringOfChar(c byte, n int) string {
	out := make([]byte, n)
	for i := range out {
		out[i] = c
	}
	return string(out)
}
