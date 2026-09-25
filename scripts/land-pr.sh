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

# GitHub computes mergeStateStatus lazily: right after another PR merges it reads UNKNOWN (or a
# stale CLEAN) for a few seconds, which once skipped a needed rebase.
sleep 5
STATE=UNKNOWN
for _ in $(seq 1 20); do
	STATE="$(gh pr view "$PR" --json mergeStateStatus -q .mergeStateStatus)"
	[ "$STATE" != "UNKNOWN" ] && break
	sleep 3
done
if [ "$STATE" = "BEHIND" ] || [ "$STATE" = "DIRTY" ]; then
	echo "-- branch is $STATE, rebasing onto main --"
	(cd "$WORKTREE" && git fetch origin main && git rebase origin/main)
	echo "-- re-verifying: $TEST_CMD --"
	(cd "$WORKTREE" && eval "$TEST_CMD")
	(cd "$WORKTREE" && git push --force-with-lease)
	echo "-- waiting for CI on the rebased commit --"
	# Wait for the run of the pushed commit itself: pending checks are not registered for a few
	# seconds after a push, so counting them once let the merge run before CI had started.
	HEAD_SHA="$(cd "$WORKTREE" && git rev-parse HEAD)"
	RUN_ID=""
	while [ -z "$RUN_ID" ]; do
		sleep 8
		RUN_ID="$(gh run list --branch "$BRANCH" --json databaseId,headSha -q ".[] | select(.headSha == \"$HEAD_SHA\") | .databaseId" | head -1)"
	done
	gh run watch "$RUN_ID" --exit-status >/dev/null
fi

echo "-- confirming a real CI run happened, not just a third-party check --"
gh run list --branch "$BRANCH" --limit 3
gh pr checks "$PR"

echo "-- merging --"
# Merge first and delete the branch after the worktree is gone: a refused merge then leaves the
# worktree in place, and the branch is not deleted while a worktree still uses it.
gh pr merge "$PR" --squash
git worktree remove "$WORKTREE" --force
git push origin --delete "$BRANCH" || true
git branch -D "$BRANCH" || true

cd "$REPO_ROOT"
git checkout main
git pull --ff-only
echo "== PR #$PR landed, worktree removed, main synced =="
