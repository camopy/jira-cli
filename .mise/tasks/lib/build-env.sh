#!/usr/bin/env bash

jira_git_value() {
	local fallback="$1"
	shift

	git "$@" 2>/dev/null || printf '%s\n' "$fallback"
}

jira_version() {
	jira_git_value dev describe --tags --always --dirty
}

jira_branch() {
	jira_git_value unknown rev-parse --abbrev-ref HEAD
}

jira_build_by() {
	whoami
}

# Version resolution is delegated to clive, so the version ldflag targets
# clive's injection point. Commit and build time are no longer injected —
# internal/version reads them from the VCS metadata Go embeds at build time.
# Branch and BuildBy stay on internal/version (clive does not model them).
jira_ldflags() {
	local module="${MODULE:?MODULE is required}"

	printf "%s" "-s -w"
	printf " -X 'github.com/gechr/clive.version=%s'" "$(jira_version)"
	printf " -X '%s/internal/version.Branch=%s'" "$module" "$(jira_branch)"
	printf " -X '%s/internal/version.BuildBy=%s'" "$module" "$(jira_build_by)"
}

jira_binary_path() {
	local dist_dir="${DIST_DIR:?DIST_DIR is required}"
	local binary_name="${BINARY_NAME:?BINARY_NAME is required}"
	local goos="${GOOS:-$(go env GOOS)}"
	local goarch="${GOARCH:-$(go env GOARCH)}"

	printf "%s/%s-%s-%s\n" "$dist_dir" "$binary_name" "$goos" "$goarch"
}
