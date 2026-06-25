package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// mrInfo holds the metadata returned by `glab mr view --output json`.
type mrInfo struct {
	ID           int    `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	WebURL       string `json:"web_url"`
}

// runGlab dispatches glab subcommands.
func runGlab(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printGlabUsage()
		return nil
	}

	switch args[0] {
	case "mr":
		return runGlabMR(args[1:])
	default:
		return fmt.Errorf("unknown glab command: %s\nRun 'ocr glab --help' for usage", args[0])
	}
}

// runGlabMR resolves a GitLab MR via glab and delegates to runReview.
//
// Usage:
//
//	ocr glab mr [<id>] [review-flags...]
//
// If <id> is omitted, glab auto-detects the MR from the current branch.
// All remaining flags are forwarded to runReview.
func runGlabMR(args []string) error {
	// Separate MR ID (first non-flag argument) from review flags.
	mrID := ""
	var reviewFlags []string
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		mrID = args[0]
		// Validate MR ID is a positive integer.
		if mrID != "" {
			if id, err := strconv.Atoi(mrID); err != nil || id <= 0 {
				return fmt.Errorf("invalid MR ID %q: must be a positive integer", mrID)
			}
		}
		reviewFlags = args[1:]
	} else {
		reviewFlags = args
	}

	// Verify glab is available.
	if _, err := exec.LookPath("glab"); err != nil {
		return fmt.Errorf("glab not found — install from https://gitlab.com/gitlab-org/cli")
	}

	// Fetch MR metadata from GitLab.
	info, err := getMRInfo(mrID)
	if err != nil {
		return fmt.Errorf("get MR info: %w", err)
	}

	// Resolve branch names to refs that git can verify.
	repoDir, err := resolveRepoDir("")
	if err != nil {
		return fmt.Errorf("resolve repo: %w", err)
	}

	fromRef := resolveBranchRef(repoDir, info.TargetBranch)
	toRef := resolveBranchRef(repoDir, info.SourceBranch)

	if fromRef == "" {
		return fmt.Errorf("cannot resolve target branch %q — is it fetched? Try: git fetch origin %s",
			info.TargetBranch, info.TargetBranch)
	}
	if toRef == "" {
		return fmt.Errorf("cannot resolve source branch %q — is it fetched? Try: git fetch origin %s",
			info.SourceBranch, info.SourceBranch)
	}

	// Strip --from/--to from user-supplied flags — these are auto-derived from the MR
	// and would cause "flag redefined" errors in parseReviewFlags.
	reviewFlags = stripFromToFlags(reviewFlags)

	// Build review arguments: --from <target> --to <source>
	reviewArgs := []string{
		"--from", fromRef,
		"--to", toRef,
	}

	// Auto-fill --background from MR title and description (unless user already provided it).
	if !hasBackgroundFlag(reviewFlags) {
		bg := strings.TrimSpace(info.Title)
		if info.Description != "" {
			desc := strings.TrimSpace(info.Description)
			if desc != "" {
				bg += "\n\n" + desc
			}
		}
		if bg != "" {
			reviewArgs = append(reviewArgs, "--background", bg)
		}
	}

	reviewArgs = append(reviewArgs, reviewFlags...)

	return runReview(reviewArgs)
}

// getMRInfo calls `glab mr view [id] --output json` and parses the result.
// When id is empty, glab auto-detects the MR from the current branch.
func getMRInfo(id string) (*mrInfo, error) {
	args := []string{"mr", "view", "--output", "json"}
	if id != "" {
		args = append(args, id)
	}

	cmd := exec.Command("glab", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr == "" {
				stderr = exitErr.String()
			}
			return nil, fmt.Errorf("glab mr view failed: %s", stderr)
		}
		return nil, fmt.Errorf("glab mr view: %w", err)
	}

	var info mrInfo
	if err := json.Unmarshal(out, &info); err != nil {
		return nil, fmt.Errorf("parse glab output: %w\nRaw output:\n%s", err, string(out))
	}

	if info.SourceBranch == "" || info.TargetBranch == "" {
		return nil, fmt.Errorf("glab returned incomplete MR data (missing source or target branch)")
	}

	return &info, nil
}

// resolveBranchRef resolves a branch name to a git ref that can be verified
// with `git rev-parse`. Tries: local branch → origin/<branch> → auto-fetch.
func resolveBranchRef(repoDir, branch string) string {
	if branch == "" {
		return ""
	}

	// 1. Try local branch.
	if refExists(repoDir, branch) {
		return branch
	}

	// 2. Try origin/<branch>.
	remote := "origin/" + branch
	if refExists(repoDir, remote) {
		return remote
	}

	// Guard against branch names that could be misinterpreted by git.
	if strings.HasPrefix(branch, "-") || strings.Contains(branch, ":") {
		return ""
	}

	// 3. Try to fetch the branch from origin.
	if out, err := runGitCmd(repoDir, "fetch", "--end-of-options", "origin", branch+":refs/remotes/origin/"+branch); err != nil {
		fmt.Fprintf(os.Stderr, "[ocr] fetch origin %s failed: %v\n%s", branch, err, string(out))
	}
	if refExists(repoDir, remote) {
		return remote
	}

	return ""
}

// refExists checks if a git ref exists via `git rev-parse --verify`.
func refExists(repoDir, ref string) bool {
	_, err := runGitCmd(repoDir, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	return err == nil
}

// hasBackgroundFlag returns true if the flag list contains --background or -b.
func hasBackgroundFlag(args []string) bool {
	for _, a := range args {
		if a == "--background" || a == "-b" {
			return true
		}
		if strings.HasPrefix(a, "--background=") || strings.HasPrefix(a, "-b=") {
			return true
		}
		// Catch condensed forms: -bvalue or --backgroundvalue (Go flag package accepts these).
		if strings.HasPrefix(a, "-b") && len(a) > 2 && !strings.HasPrefix(a, "-b=") {
			return true
		}
		if strings.HasPrefix(a, "--background") && len(a) > len("--background") && !strings.HasPrefix(a, "--background=") {
			return true
		}
	}
	return false
}

// stripFromToFlags removes --from <value> and --to <value> pairs from the argument list.
// These are auto-derived from the MR and would cause "flag redefined" errors if forwarded.
func stripFromToFlags(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--from" || args[i] == "--to" {
			// Skip the flag and its value.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		}
		if strings.HasPrefix(args[i], "--from=") || strings.HasPrefix(args[i], "--to=") {
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func printGlabUsage() {
	fmt.Println(`OpenCodeReview - GitLab MR Review

Usage:
  ocr glab mr [<id>] [review-flags...]

Description:
  Review a GitLab merge request by ID, using glab to fetch MR metadata
  and diff. The MR's target branch is used as --from, and the source
  branch as --to. All other review flags are forwarded to ocr review.

Examples:
  ocr glab mr 123                            Review MR #123
  ocr glab mr                                Auto-detect MR from current branch
  ocr glab mr 123 --model opus-4-6           Override the LLM model
  ocr glab mr 123 --preview                  Preview changed files
  ocr glab mr 123 --background "extra ctx"   Add extra context

Prerequisites:
  glab CLI installed and authenticated: https://gitlab.com/gitlab-org/cli
  ocr CLI with a configured LLM provider.

Use "ocr review -h" for additional review flags.`)
}
