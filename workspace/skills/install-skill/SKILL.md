---
name: install-skill
description: Install a directory-based Agent Skill from a trusted source into the workspace skills directory. Activate only when the user explicitly asks to install a Skill.
compatibility: Installed skills become available after OctoSucker restarts and rebuilds its validated tool catalog.
allowed-tools: get_skills_root_dir run_command
metadata:
  version: "2"
---

# Install Skill

Install only when the user explicitly requests it and the source is trusted.

1. Call `get_skills_root_dir` to locate the workspace skills root.
2. A skill must be a directory named exactly like its frontmatter `name` and must contain `SKILL.md`.
3. Supporting files may live under `references/`, `scripts/`, or `assets/`.
4. Do not install a standalone top-level Markdown file.
5. Do not follow symlinks or write outside the returned skills root.
6. OctoSucker validates skills at startup. After installation, tell the user that the service must restart before the skill enters the catalog.

For a GitHub blob URL, use the equivalent raw URL. Prefer installing a complete repository subdirectory or archive when the skill has supporting resources; downloading only `SKILL.md` may produce an incomplete skill.
