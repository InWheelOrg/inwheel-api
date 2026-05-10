---
description: Guide for creating GitHub issues in InWheel repositories. Use this when filing, creating, or drafting a new GitHub issue for a bug, feature, or task.
metadata:
    github-path: creating-issues
    github-ref: refs/heads/main
    github-repo: https://github.com/InWheelOrg/skills
    github-tree-sha: 48ae949d9118aa4057b678672aca405e60dab2cc
name: creating-issues
---
## Issue title

- Plain sentence — no conventional commits prefix (`feat:`, `fix:`, etc.)
- No dashes in the title
- Describes what needs to be done or what the problem is, clearly and concisely
- Lowercase unless a proper noun or acronym demands otherwise

Good: `PBF reader that streams OSM elements from a .pbf file`
Bad: `feat: pbf-reader — stream OSM elements`

## Issue body

Use this structure:

```markdown
## Summary
One or two sentences expanding on the title. What is this issue about?

## Context
Why is this needed? Background, motivation, or relevant constraints.
Omit this section if the summary is self-explanatory.

## Scope
- Bullet list of what this issue covers
- And what is explicitly out of scope if non-obvious

## Acceptance criteria
- [ ] Checkbox for each concrete condition that marks this issue as done
- [ ] Written from the perspective of observable, verifiable outcomes

## Notes
Implementation hints, links, gotchas, open questions.
Omit this section if there is nothing worth adding.
```

## Labels

Always apply all three label types unless there is a clear reason not to:

| Category | Examples |
|---|---|
| Type | `enhancement`, `bug`, `documentation`, `dependencies` |
| Service | `service:ingestion-service`, `service:inwheel-api` |
| Priority | `priority:must`, `priority:should`, `priority:future` |

## CLI command

```bash
gh issue create \
  --repo OWNER/REPO \
  --title "Clear sentence title here" \
  --body "..." \
  --label "enhancement,service:ingestion-service,priority:must"
```
