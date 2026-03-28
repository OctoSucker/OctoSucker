# Skill: Install Skill From URL

## Goal
Install a skill markdown file from a URL and load it.

## Steps

1. Get skills root directory via tool:
   - capability: `skills`
   - tool: `get_skills_root_dir`
   Save returned `skills_root_dir` as `<skills_dir>`.

2. If URL is a GitHub blob link, convert it to raw URL:
   - `https://github.com/<org>/<repo>/blob/<branch>/<path>`
   - -> `https://raw.githubusercontent.com/<org>/<repo>/<branch>/<path>`

3. Use `exec.run_command` with `work_dir` (relative to workspace) and relative file paths:
   - create folder for the skill
   - download skill file into that folder as `SKILL.md`
   - example shell flow:
     - `mkdir -p <skill_name>`
     - `curl -L <raw_url> -o <skill_name>/SKILL.md`
   Do not use host absolute paths like `/Users/...`.

4. Optionally verify:
   - `cat <skill_name>/SKILL.md`

5. Reload skills:
   - capability: `skills`
   - tool: `reload_skills`