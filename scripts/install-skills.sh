#!/usr/bin/env bash
# Installs the agent skills this repo needs, and nothing else.
#
# Re-running it refreshes every listed skill to the latest upstream version.
# Review the result with `git diff skills-lock.json .agents/skills` before
# committing: a skill update changes agent behaviour, so it is reviewed like code.
set -euo pipefail

cd "$(dirname "$0")/.."

# Why each skill is here: README.md, "Agent skills".
SKILLS=(
  setup-matt-pocock-skills
  implement
  tdd
  codebase-design
  code-review
  diagnosing-bugs
)

# Skills not from SOURCE: written in this repo, or installed from another source
# (refresh those by name with `npx skills@1.7.4 update -p <name>`). The extra-skill check below allows them.
OTHER_SKILLS=()

SOURCE="mattpocock/skills"
CLI="skills@1.7.4" # pinned: the installer runs with your credentials, so it is upgraded on purpose, not silently

# The CLI sends anonymous usage data by default.
export DISABLE_TELEMETRY=1

if [ "${#SKILLS[@]}" -eq 0 ]; then
  echo "install-skills: the skill list is empty" >&2
  exit 1
fi

args=()
for s in "${SKILLS[@]}"; do args+=(--skill "$s"); done

npx -y "$CLI" add "$SOURCE" "${args[@]}" --agent claude-code --agent universal --yes

# Check what landed on disk, not the installer's exit code.
missing=0
for s in "${SKILLS[@]}"; do
  for dir in .agents/skills .claude/skills; do
    if [ ! -f "$dir/$s/SKILL.md" ]; then
      echo "install-skills: missing $dir/$s/SKILL.md" >&2
      missing=1
    fi
  done
done

extra=0
allowed=" ${SKILLS[*]} ${OTHER_SKILLS[*]:-} "
for path in .agents/skills/*/ .claude/skills/*/; do
  [ -e "$path" ] || continue
  name="$(basename "$path")"
  case "$allowed" in
    *" $name "*) ;;
    *) echo "install-skills: unexpected skill $path (add it to SKILLS or OTHER_SKILLS, or remove it)" >&2; extra=1 ;;
  esac
done

if [ "$missing" -ne 0 ] || [ "$extra" -ne 0 ]; then
  exit 1
fi

echo "install-skills: ${#SKILLS[@]} skills installed from $SOURCE"
