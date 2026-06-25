---
name: open-code-mr-review
description: >
  Performs AI-powered code review on GitLab merge requests using `glab mr`
  and `ocr glab mr`. Use when the user asks to review a GitLab MR, review
  a merge request by ID/URL, or review an MR from the current branch.
  Produces line-level review comments and can automatically apply fixes
  when requested. With appropriate review rules, can detect various types
  of issues including bugs, security vulnerabilities, performance problems,
  and code quality concerns.
license: Apache-2.0
compatibility: >
  Requires `glab` CLI installed and authenticated (`glab auth login`).
  Requires `ocr` CLI installed with a configured LLM (Anthropic or
  OpenAI-compatible).
metadata:
  author: community
  homepage: https://github.com/alibaba/open-code-review
  version: "1.0.0"
---

# Open Code MR Review

A skill for reviewing GitLab merge requests using [open-code-review](https://github.com/alibaba/open-code-review) (`ocr`) + [glab](https://gitlab.com/gitlab-org/cli).

## Prerequisites check

Before starting a review, verify the environment:

```bash
# 1. Check glab is installed and authenticated
which glab || echo "NOT INSTALLED"
glab auth status

# 2. Check ocr is installed
which ocr || echo "NOT INSTALLED"

# 3. Verify LLM connectivity
ocr llm test
```

If `glab` is not installed:

```bash
# macOS
brew install glab

# Linux — see https://gitlab.com/gitlab-org/cli/-/releases
```

If `glab` is not authenticated:

```bash
glab auth login
```

If `ocr` is not installed:

```bash
npm install -g @alibaba-group/open-code-review
```

If `ocr llm test` fails, guide the user to configure an LLM provider — see the `open-code-review` skill for details.

Stop here and ask the user to resolve missing prerequisites — never invent or hardcode credentials.

## Workflow

### Step 1: Gather Business Context

Analyze the MR title, description, and linked issues to extract concise business context. Pass additional context via `--background` to improve review quality.

### Step 2: Run MR Review

Run the review with `ocr glab mr`. **Always pass `--audience agent`** for clean output:

```bash
ocr glab mr <id> --audience agent
```

**MR ID handling:**

- **Explicit ID**: `ocr glab mr 123` — review MR #123
- **Auto-detect**: `ocr glab mr` — glab detects MR from the current branch
- **From URL**: Extract the ID from a GitLab MR URL like `https://gitlab.com/OWNER/REPO/-/merge_requests/123`

**Additional flags** (forwarded to `ocr review`):

| Flag | Purpose |
|------|---------|
| `--background "ctx"` / `-b "ctx"` | Add extra business context |
| `--model <name>` | Override the LLM model |
| `--preview` / `-p` | Preview changed files without LLM |
| `--concurrency <n>` | Max concurrent file reviews (default: 8) |
| `--timeout <minutes>` | Per-file timeout (default: 10) |
| `--format json` | JSON output for programmatic consumption |

**Common invocation patterns:**

| User says | Command to run |
|-----------|---------------|
| "review MR 123" | `ocr glab mr 123 --audience agent` |
| "review this MR" | `ocr glab mr --audience agent` |
| "review MR 123 with GPT-4" | `ocr glab mr 123 --audience agent --model gpt-4` |
| "what files changed in MR 123?" | `ocr glab mr 123 --preview` |

### Step 3: Classify and Report

For each comment from the review output, classify by priority and report all issues to the user:

- **High**: Obvious bugs, security issues, clear mistakes, or well-founded suggestions with precise fix proposals
- **Medium**: Reasonable concerns but context-dependent, style/performance suggestions, or fixes that require manual implementation
- **Low**: Discarded silently (likely false positives, lacking context, nitpicks, or meaningless suggestions)

Report all comments grouped by priority level. **All output (issue descriptions, recommendations, summaries) must be in Chinese (简体中文).**

### Step 4: Fix (only on explicit request)

**Default behavior: DO NOT apply any fixes.** Only report issues and let the user decide what to do.

Only proceed with fixes when the user explicitly requests it with phrases like:

- "review and fix"
- "apply the fixes"
- "fix the issues"
- or similar explicit fix intent

When the user explicitly requests fixes:

- Focus on High and Medium priority items
- Apply fixes directly to the code when safe and well-defined
- For complex fixes requiring manual intervention, clearly describe what needs to be done
- Always verify fixes with the user before committing

If the user only requested "review" without clear fix intent, **stop after Step 3** — do not ask whether to fix, do not suggest applying fixes. Simply present the review results and wait for the user's next instruction.

## How It Works

`ocr glab mr` resolves an MR to a branch comparison internally:

```
ocr glab mr <id>
  → glab mr view <id> --output json  (get source_branch, target_branch, title, description)
  → ocr review --from <target_branch> --to <source_branch> --background "<title>\n\n<description>"
```

The MR title and description are automatically passed as `--background` context unless the user explicitly provides their own `--background` or `-b` flag.

Branch resolution tries: local branch → `origin/<branch>` → auto-fetch from origin.

## Output Format

Each comment contains:

- `path`: File path
- `content`: Review comment text
- `start_line` / `end_line`: Line range (both 0 means positioning failed)
- `suggestion_code`: Optional fix suggestion
- `existing_code`: Optional original code snippet
- `thinking`: Optional LLM reasoning process

After filtering comments by priority, present results using this template:

```markdown
## Code Review Results — MR !<id>: <title>

**Files reviewed**: N
**Issues found**: X high priority / Y medium priority

### High Priority

- **`path/to/file.java:42`** — Brief description
  > Recommendation: How to fix

### Medium Priority

- **`path/to/file.ts:88`** — Brief description
  > Recommendation: How to fix (if applicable)
```

If the review found no issues after filtering, simply state: "Review complete — no issues found in N files."

## Gotchas

- **glab must be authenticated** — run `glab auth status` to verify before starting.
- **Remote branches may need fetching** — the command auto-fetches the source branch if not found locally, but this requires network access.
- **Working directory matters** — `ocr glab mr` operates on the Git repo at the current directory. Use `--repo /path/to/repo` to run from elsewhere.
- **LLM must be configured first** — `ocr review` will fail loudly if no LLM is reachable. Always run `ocr llm test` before the first review.
- **Don't pass `--audience human`** — it streams progress UI that pollutes output. Always use `--audience agent`.
- **Comment language follows config** — set `language` config to `English` or `Chinese` (default: Chinese) to control review comment language.

## References

- Full docs: https://github.com/alibaba/open-code-review
- glab CLI: https://gitlab.com/gitlab-org/cli
- NPM package: https://www.npmjs.com/package/@alibaba-group/open-code-review
