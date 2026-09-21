#!/usr/bin/env bash
# Run once, after `git remote add origin ...` and the first push, by someone
# with admin on the repo. Requires `gh auth login` already done.
set -euo pipefail

REPO="${1:?usage: scripts/setup-branch-protection.sh <owner>/<repo>}"

gh api "repos/$REPO/branches/main/protection" \
	--method PUT \
	-F "required_status_checks[strict]=true" \
	-f "required_status_checks[contexts][]=contracts" \
	-f "required_status_checks[contexts][]=pr-title" \
	-f "required_status_checks[contexts][]=lint-test-iss" \
	-f "required_status_checks[contexts][]=lint-test-gw" \
	-f "required_status_checks[contexts][]=lint-test-web" \
	-f "required_status_checks[contexts][]=lint-test-set" \
	-F "enforce_admins=true" \
	-F "required_pull_request_reviews[required_approving_review_count]=1" \
	-F "restrictions=null"

gh api "repos/$REPO" \
	--method PATCH \
	-F "allow_squash_merge=true" \
	-F "allow_merge_commit=false" \
	-F "allow_rebase_merge=false"

echo "Branch protection + squash-only merge set on $REPO"
