#!/usr/bin/env bash
# merge-branch.sh — conditional merge strategy for delegated-implement
#
# Usage: merge-branch.sh <original-branch> <impl-branch> <commit-message>
#
# Single commit on branch  → squash merge (linear history)
# Multiple commits         → merge --no-ff (preserve branch topology)

set -euo pipefail

original_branch="$1"
impl_branch="$2"
commit_msg="$3"

git checkout "$original_branch"

commit_count=$(git rev-list --count "${original_branch}..${impl_branch}")

if [ "$commit_count" -eq 1 ]; then
  git merge --squash "$impl_branch"
  git commit -m "$commit_msg"
else
  git merge --no-ff "$impl_branch" -m "$commit_msg"
fi

git branch -d "$impl_branch"
