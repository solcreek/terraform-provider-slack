#!/bin/sh
# SPDX-License-Identifier: MPL-2.0
# Keep registry-indexed versions immutable, following Solcreek's release policy.
set -eu
repo=solcreek/terraform-provider-slack
case "${1:-}" in
  --apply) apply=true ;;
  '') apply=false ;;
  *) echo "Usage: $0 [--apply]" >&2; exit 2 ;;
esac

enabled=$(gh api "repos/$repo/immutable-releases" --jq .enabled)
if [ "$enabled" != true ] && [ "$apply" = true ]; then
  gh api --method PUT "repos/$repo/immutable-releases"
  enabled=true
fi

id=$(gh api "repos/$repo/rulesets" --jq '.[] | select(.name == "release-tags") | .id')
if [ "$apply" = true ]; then
  if [ -n "$id" ]; then
    gh api --method PUT "repos/$repo/rulesets/$id" --input .github/release-tags.json >/dev/null
  else
    id=$(gh api --method POST "repos/$repo/rulesets" --input .github/release-tags.json --jq .id)
  fi
fi

if [ "$enabled" != true ] || [ -z "$id" ]; then
  echo "Release protections are missing. Run $0 --apply." >&2
  exit 1
fi

filter='{name,target,enforcement,bypass_actors,conditions:{ref_name:.conditions.ref_name},rules:[.rules[]|{type}]|sort_by(.type)}'
actual=$(gh api "repos/$repo/rulesets/$id" | jq -Sc "$filter")
expected=$(jq -Sc "$filter" .github/release-tags.json)
if [ "$actual" != "$expected" ]; then
  echo "Release tag rules differ from .github/release-tags.json." >&2
  exit 1
fi
echo "Immutable releases and release tag protections are configured."
