---
name: github-release-notes
description: Draft GitHub release notes from selected commits in a local git repository. Use when Codex needs to turn specific `git log` entries, commit SHAs, or revision ranges into a release summary with a short "What's Changed" narrative, grouped highlight bullets, and a full changelog.
---

# GitHub Release Notes

Generate release notes from a user-selected set of commits. Keep the output concise, readable, and suitable for a GitHub release body.

## Workflow

1. Determine the exact commits to include.
2. Run `scripts/collect_commits.py` with the selected SHAs or ranges.
3. Read the emitted commit list or JSON payload.
4. Write release notes in this structure unless the user asks for another format:

```md
## What's Changed

This release ...

Highlights:

- ...
- ...

## Full Changelog

- feat(scope): subject (abcdef1)
- fix: subject (1234567)
```

## Commit Collection

Use the helper script instead of reformatting `git log` manually.

```bash
python3 ~/.codex/skills/github-release-notes/scripts/collect_commits.py \
  --repo /path/to/repo \
  --format json \
  2bb0481 1db7019 16258ce 9afdd2c 9b85fd8 9b6969e
```

Useful patterns:

- Pass individual SHAs to preserve a curated order.
- Pass a range like `base..head` to include everything in commit order.
- Use `--format markdown` when the full changelog bullets are all you need.

## Writing Rules

- Open with a single plain-English sentence that summarizes the release at a theme level.
- Add `Highlights:` and 2-5 bullets when there is more than one notable change area.
- Merge related commits into one highlight when they clearly describe the same user-facing improvement or fix.
- Keep highlight bullets outcome-focused. Prefer "Added agent log support across the CLI and API" over copying commit subjects verbatim.
- Keep the `Full Changelog` exhaustive for the selected commits.
- Preserve commit order from the helper output.
- Format each full changelog item as `- subject (shortsha)`.
- Do not invent changes that are not supported by the selected commits.
- If the commits are too small or mechanical for a narrative summary, say so briefly and keep the output minimal.

## Heuristics

- Group by user-visible area first, then by subsystem, then by maintenance work.
- Treat `feat`, `fix`, and meaningful UI polish as highlight candidates.
- Usually leave `chore` items out of `Highlights` unless they materially affect operators or release behavior.
- When one commit clearly explains another small cleanup commit, combine them into one highlight.
- Preserve important qualifiers such as CLI, API, web UI, runtime, shutdown, rendering, or logs.

## Example Prompt

`Use $github-release-notes to draft release notes from commits 2bb0481 1db7019 16258ce 9afdd2c 9b85fd8 9b6969e in the current repo.`
