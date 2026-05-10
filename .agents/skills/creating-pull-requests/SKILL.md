---
description: Guide for creating GitHub pull requests in InWheel repositories. Use this when opening, creating, or drafting a pull request or PR.
metadata:
    github-path: creating-pull-requests
    github-ref: refs/heads/main
    github-repo: https://github.com/InWheelOrg/skills
    github-tree-sha: d7d256f4530e6b78a1fa1ae2c3eda5bd4cb6ef1f
name: creating-pull-requests
---
## PR title

The PR title mirrors the commit subject line exactly — same type prefix, same description:

```
feat: PBF reader streaming OSM elements from .pbf files
```

## PR body (non-draft)

```markdown
## What
One or two sentences describing what this PR does and why.

## Changes
- Bullet list of what was done, using the same imperative verb style as commits
- One bullet per meaningful change
- Match the detail level of the commit body

## Notes
Any reviewer-facing context: gotchas, trade-offs, follow-up work, known limitations.
Omit this section entirely if there is nothing worth noting.

Closes #N
```

- `Closes #N` goes at the very end, after all sections, on its own line with a blank line before it
- Use `Part of #N` instead of `Closes #N` if the PR only partially addresses the issue

## Labels and assignee

Always include:
- Labels: same three-category system as issues (type + service + priority)
- Assignee: `--assignee @me`

## Draft PRs

Draft PRs are free-form. No strict body structure required. Open as draft when asked to or when the work is exploratory or incomplete.

## CLI command

```bash
gh pr create \
  --repo OWNER/REPO \
  --title "feat: short description" \
  --body "..." \
  --label "enhancement,service:ingestion-service,priority:must" \
  --assignee "@me"
```

For a draft:

```bash
gh pr create \
  --repo OWNER/REPO \
  --title "..." \
  --body "..." \
  --draft
```
