#!/usr/bin/env bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "Usage: scripts/release.sh vX.Y.Z" >&2
  exit 2
fi

version="$1"

if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Version must match vX.Y.Z, for example v0.1.0" >&2
  exit 2
fi

if [ "$(git branch --show-current)" != "main" ]; then
  echo "Release must run from main" >&2
  exit 1
fi

if [ -n "$(git status --porcelain)" ]; then
  echo "Working tree must be clean before release" >&2
  exit 1
fi

git fetch --quiet origin main --tags

if ! git merge-base --is-ancestor origin/main HEAD; then
  echo "Local main is behind or diverged from origin/main" >&2
  exit 1
fi

if git rev-parse --verify --quiet "refs/tags/$version" >/dev/null; then
  echo "Tag already exists: $version" >&2
  exit 1
fi

if git ls-remote --exit-code --tags origin "refs/tags/$version" >/dev/null 2>&1; then
  echo "Remote tag already exists: $version" >&2
  exit 1
fi

make check

git push origin main
git tag -a "$version" -m "Release $version"
git push origin "$version"

echo "Pushed $version. GitHub Actions will create the release."
