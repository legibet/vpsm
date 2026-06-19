#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "Usage: scripts/release.sh vX.Y.Z"
}

if [ "$#" -ne 1 ]; then
  usage
  exit 2
fi

version="$1"

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Version must match vX.Y.Z, for example v0.1.0" >&2
  exit 2
fi

current_branch="$(git branch --show-current)"
if [ -z "$current_branch" ]; then
  echo "Release must run from a branch, not a detached HEAD" >&2
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "Working tree must be clean before release" >&2
  exit 1
fi

if git rev-parse --verify --quiet "$version" >/dev/null; then
  echo "Tag already exists: $version" >&2
  exit 1
fi

if git ls-remote --exit-code --tags origin "refs/tags/$version" >/dev/null 2>&1; then
  echo "Remote tag already exists: $version" >&2
  exit 1
fi

make check

git push origin "$current_branch"
git tag -a "$version" -m "Release $version"
git push origin "$version"

echo "Pushed $version. GitHub Actions will create the release."
