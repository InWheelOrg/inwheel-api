---
description: Guide for writing git commit messages in InWheel repositories. Use this when writing a commit message, staging changes, or running git commit.
metadata:
    github-path: committing
    github-ref: refs/heads/main
    github-repo: https://github.com/InWheelOrg/skills
    github-tree-sha: 1344034c0fe1796c26c4377de17acba8e6efbccc
name: committing
---
## Commit message format

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/). Every commit has a subject line and an optional body.

### Subject line

```
<type>: <short imperative description>
```

- **type** must be one of: `feat`, `fix`, `chore`, `docs`, `refactor`, `test`, `ci`, `perf`, `build`
- Description is lowercase, no period at the end
- Keep it under 72 characters
- Use the imperative mood: "add X", not "adds X" or "added X"

### Body (optional but encouraged for non-trivial commits)

Leave a blank line after the subject, then:

1. **Optional free-form paragraph** — if the change is large or the reasoning is non-obvious, write 1–3 sentences explaining *what* changed and *why*. This comes before the bullet list.
2. **Bullet list** — one bullet per meaningful change. Start each bullet with an imperative verb: `Add`, `Create`, `Remove`, `Fix`, `Update`, `Rename`, `Extract`, `Replace`, `Delete`, `Move`, `Refactor`.

### Rules

- No `Co-authored-by` trailers
- No `Closes #N` references — those go in the pull request body, not the commit
- If a commit only touches one file or is a single obvious change, a subject line alone is enough — no body needed

### Examples

Simple:
```
fix: handle missing denseinfo when parsing omitmeta PBF files
```

With body and bullets:
```
feat: PBF reader streaming OSM elements from .pbf files

Handles all OSM element types with correct delta decoding. Filter is
injected as a function type so the reader is testable without real files.

- Add OsmPbfParser extending BinaryParser with delta decoding for dense nodes, ways, and relations
- Add PbfReader as the public facade with file validation and PbfReadException wrapping
- Add PbfReadException as the domain exception for read failures
- Add 8 unit tests covering all parser paths including negative deltas and mixed tags
```

Chore:
```
chore: bump kotlinx-serialization to 1.11.0
```
