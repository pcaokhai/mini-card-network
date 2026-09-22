#!/usr/bin/env bash
# Rebase (if needed), re-verify, merge one PR, and clean up its worktree — the same
# 8-step dance that got done by hand ~15 times across Sprints 2-4, folded into one call.
#
# Usage: scripts/land-pr.sh <pr-number> <worktree-path> <test-command>
#
# <test-command> re-verifies the branch after a rebase (skipped entirely if no rebase was
# needed — CI already proved that exact commit green, a rebase is the only thing that can
# change the code under test). Examples:
#   scripts/land-pr.sh 42 ../mcn-worktrees/iss-301-schema-seed \
#     "cd issuer-jpos && ./gradlew spotlessCheck build"
#   scripts/land-pr.sh 43 ../mcn-worktrees/gw-303-purchase-flow \
#     "cd gateway-go && CGO_ENABLED=0 go build ./... && CGO_ENABLED=0 go test -count=1 ./..."
#   scripts/land-pr.sh 44 ../mcn-worktrees/web-305-pos-simulator \
#     "cd web-next && pnpm lint && pnpm test && pnpm build"
#
# Uses `gh` (not `gh-axi`) for state checks that need exact JSON parsing; interactive
# inspection outside this script should still prefer `gh-axi` per CLAUDE.md §5.

set -euo pipefail

PR="${1:?usage: scripts/land-pr.sh <pr-number> <worktree-path> <test-command>}"
WORKTREE="${2:?usage: scripts/land-pr.sh <pr-number> <worktree-path> <test-command>}"
TEST_CMD="${3:?usage: scripts/land-pr.sh <pr-number> <worktree-path> <test-command>}"

REPO_ROOT="$(git rev-parse --show-toplevel)"
BRANCH="$(gh pr view "$PR" --json headRefName -q .headRefName)"

echo "== PR #$PR ($BRANCH) =="

STATE="$(gh pr view "$PR" --json mergeStateStatus -q .mergeStateStatus)"
if [ "$STATE" = "BEHIND" ] || [ "$STATE" = "DIRTY" ]; then
	echo "-- branch is $STATE, rebasing onto main --"
	(cd "$WORKTREE" && git fetch origin main && git rebase origin/main)
	echo "-- re-verifying: $TEST_CMD --"
	(cd "$WORKTREE" && eval "$TEST_CMD")
	(cd "$WORKTREE" && git push --force-with-lease)
	echo "-- waiting for CI on the rebased commit --"
	while [ "$(gh pr checks "$PR" 2>&1 | grep -c 'pending')" != "0" ]; do sleep 8; done
fi

echo "-- confirming a real CI run happened, not just a third-party check --"
gh run list --branch "$BRANCH" --limit 3
gh pr checks "$PR"

echo "-- merging --"
git worktree remove "$WORKTREE" --force
gh pr merge "$PR" --squash --delete-branch

cd "$REPO_ROOT"
git checkout main
git pull --ff-only
echo "== PR #$PR landed, worktree removed, main synced =="
