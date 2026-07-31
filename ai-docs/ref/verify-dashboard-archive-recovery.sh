#!/bin/sh
# Verify the archive and evidence recovery after source refs are removed.
set -eu

archive_tag=archive/ws-dashboard
source_ref="$archive_tag^2"
source_path=ai-docs/mental-model/ws-dashboard-agent-harness.md
reference_path=ai-docs/ref/agent-harness-capability-tiers.md
expected_parents='0a688af1693c8d08c0c133aed2faba0b89356963 5d5f6ade126c880b9aeae667a4314259e4892770 7f2c8c58037633eb13124075b7cc76026dd666df 17a1023f5a45b5e523f8be500bd46ac59edc06f6'
expected_main=0a688af1693c8d08c0c133aed2faba0b89356963

normalize() {
  tr '\n' ' ' | sed -E 's/[[:space:]]+/ /g; s/^ //; s/ $//'
}

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  exit 1
}

[ "$(git cat-file -t "$archive_tag")" = tag ] || fail "$archive_tag is not an annotated tag"
[ "$(git show -s --format=%P "$archive_tag^{commit}")" = "$expected_parents" ] || fail 'archive parents differ from the captured ordered tips'

archive_commit=$(git rev-parse "$archive_tag^{commit}")
archive_tag_object=$(git rev-parse "$archive_tag")
[ "$(git rev-parse "$expected_main^{tree}")" = "$(git rev-parse "$archive_tag^{commit}^{tree}")" ] || fail 'archive tree differs from the captured pre-sweep main'
remote_tag=$(git ls-remote --tags origin "refs/tags/$archive_tag" |
  awk -v ref="refs/tags/$archive_tag" '$2 == ref { print $1 }')
remote_peeled=$(git ls-remote --tags origin "refs/tags/$archive_tag^{}" |
  awk -v ref="refs/tags/$archive_tag^{}" '$2 == ref { print $1 }')
[ "$remote_tag" = "$archive_tag_object" ] || fail 'origin does not advertise the annotated archive tag object'
[ "$remote_peeled" = "$archive_commit" ] || fail 'origin does not advertise the archive tag peeled to its commit'

source_tiers=$(git show "$source_ref:$source_path" |
  sed -n '/^- Any future interactive agent-harness provider\/session work/,/  tier\./p' |
  normalize |
  sed -E \
    -e 's/^-[[:space:]]+Any future interactive agent-harness provider\/session work \(Codex app-server, OpenCode ACP, Claude CLI headless stream-json duplex\) must classify each /Interactive agent-harness provider\/session work must classify each /' \
    -e 's/the dashboard composes/an integration layer composes/' \
    -e 's/(.*tier\.).*/\1/')
reference_tiers=$(sed -n '/^Interactive agent-harness provider\/session work/,/never conflated with tier\.$/p' "$reference_path" | normalize)
[ "$source_tiers" = "$reference_tiers" ] || fail 'recovered tier definition differs from the archived source beyond the documented actor-neutral substitution'

source_codex=$(git show "$source_ref:$source_path" |
  sed -n '/\*\*Codex'"'"'s column is/,/functionality around it\.$/p' |
  normalize |
  sed -E \
    -e "s/^.*(\\*\\*Codex's column is)/\\1/" \
    -e 's/(.*functionality around it\.).*/\1/')
reference_codex=$(sed -n '/^\*\*Codex'"'"'s column is/,/functionality around it\.$/p' "$reference_path" | normalize)
[ "$source_codex" = "$reference_codex" ] || fail 'recovered Codex evidence differs from the archived source'

recovery_dir=$(mktemp -d)
trap 'rm -rf "$recovery_dir"' EXIT HUP INT TERM
git clone --quiet --no-checkout "$(git remote get-url origin)" "$recovery_dir"
git -C "$recovery_dir" checkout --quiet "$archive_tag^{commit}"
[ -d "$recovery_dir/ws-dashboard" ] || fail 'fresh archive checkout does not contain ws-dashboard/'

printf 'PASS: annotated archive tag, ordered parents, main tree, remote tag, and peeled entry verified\n'
printf 'PASS: a fresh remote archive checkout contains ws-dashboard/\n'
printf 'PASS: recovered tier definition and Codex evidence match the archived source\n'
