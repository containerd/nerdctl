#!/usr/bin/env bash

#   Copyright The containerd Authors.

#   Licensed under the Apache License, Version 2.0 (the "License");
#   you may not use this file except in compliance with the License.
#   You may obtain a copy of the License at

#       http://www.apache.org/licenses/LICENSE-2.0

#   Unless required by applicable law or agreed to in writing, software
#   distributed under the License is distributed on an "AS IS" BASIS,
#   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#   See the License for the specific language governing permissions and
#   limitations under the License.

# Exercises the parts of Soigneur that running a report locally does not: the workflow filter,
# the log parser, the renderer, and the validators. Everything here is offline - the API is
# stubbed, and the fixtures are written by the test itself.
#
# Usage:
#   ./test.sh

set -o errexit -o errtrace -o functrace -o nounset -o pipefail
# `root` belongs to the scripts this sources, which declare it readonly.
here="$(cd "$(dirname "${BASH_SOURCE[0]:-$PWD}")" 2>/dev/null 1>&2 && pwd)"
readonly here

export SOIGNEUR_REPO="owner/name"

# Sourcing the collector brings in its functions, and lib.sh with them.
# shellcheck source-path=SCRIPTDIR
. "$here"/flaky-report.sh

tmp="$(mktemp -d)"
# shellcheck disable=SC2064
trap "rm -rf '$tmp'" EXIT

failed=0
total=0

check(){
  local name="$1"
  local want="$2"
  local got="$3"

  total="$((total + 1))"
  if [ "$want" == "$got" ]; then
    echo "ok   $name"
  else
    failed="$((failed + 1))"
    echo "FAIL $name"
    echo "     want: $want"
    echo "     got:  $got"
  fi
}

# --- the workflow filter -------------------------------------------------------------------
# Regression test for #5216: `index` evaluates its argument against its own input, so filtering
# the runs used to die with "Cannot index array with string" as soon as `workflows` was set -
# which is to say, in the only configuration that ships.
cat > "$tmp"/runs.json <<'EOF'
{"workflow_runs":[
  {"id":1,"run_attempt":1,"path":".github/workflows/test.yml","created_at":"2026-09-17T00:00:00Z"},
  {"id":2,"run_attempt":2,"path":".github/workflows/lint.yml","created_at":"2026-09-17T00:00:00Z"},
  {"id":3,"run_attempt":1,"path":".github/workflows/flaky.yml","created_at":"2026-09-17T00:00:00Z"}]}
EOF

soigneur::api(){ cat "$tmp"/runs.json; }

filtered(){
  SOIGNEUR_WORKFLOWS="$1" soigneur::runs "2026-09-10T00:00:00Z" 2>&1 | jq -sc '[.[].workflow]'
}

check "no filter keeps every run" \
  '["test.yml","lint.yml","flaky.yml"]' "$(filtered "")"
check "a filter keeps the named workflows" \
  '["test.yml","flaky.yml"]' "$(filtered "test.yml,flaky.yml")"
check "a filter tolerates spaces around the names" \
  '["test.yml","flaky.yml"]' "$(filtered " test.yml , flaky.yml ")"
check "a filter matching nothing keeps nothing" \
  '[]' "$(filtered "nope.yml")"
check "attempts and ids are carried over" \
  '[["1",1],["2",2],["3",1]]' \
  "$(SOIGNEUR_WORKFLOWS="" soigneur::runs "x" | jq -sc '[.[] | [.id, .attempts]]')"

# --- the log parser ------------------------------------------------------------------------
mkdir -p "$tmp"/logs
lookup="$(printf 'inhostrootfullinux\t77\n')"

{
  echo "2026-09-17T00:00:00.0000000Z ##[group]$soigneur_marker failing"
  echo "2026-09-17T00:00:00.0000000Z $soigneur_marker failing TestAlpha"
  echo "2026-09-17T00:00:00.0000000Z $soigneur_marker flaky TestBeta/sub_one"
  echo "2026-09-17T00:00:00.0000000Z ##[endgroup]"
} > "$tmp"/logs/'1_in-host _ rootful linux.txt'

check "markers are read back, per class" \
  '{"id":"77","kind":"failing","tests":["TestAlpha"]} {"id":"77","kind":"flaky","tests":["TestBeta/sub_one"]}' \
  "$(soigneur::logs::parse "$tmp"/logs "$lookup" 2>/dev/null | tr '\n' ' ' | sed 's/ $//')"

# The logs that predate the markers only hold the block printed at the end of a job.
{
  echo "2026-09-17T00:00:00.0000000Z === Failing tests ==="
  echo "2026-09-17T00:00:00.0000000Z TestGamma"
  echo "2026-09-17T00:00:00.0000000Z ====================="
  echo "2026-09-17T00:00:00.0000000Z Post job cleanup."
} > "$tmp"/logs/'1_in-host _ rootful linux.txt'

check "the pre-marker block is still read, and stops at its rule" \
  '{"id":"77","kind":"failing","tests":["TestGamma"]}' \
  "$(soigneur::logs::parse "$tmp"/logs "$lookup" 2>/dev/null)"

# A job log holds whatever the tests printed, and what is collected ends up in an issue.
{
  echo "2026-09-17T00:00:00.0000000Z $soigneur_marker failing TestLegit"
  echo "2026-09-17T00:00:00.0000000Z $soigneur_marker failing x\`](https://evil.example)[\`y"
  echo "2026-09-17T00:00:00.0000000Z $soigneur_marker flaky Test|Pipe"
  echo "2026-09-17T00:00:00.0000000Z $soigneur_marker failing <img/src=x>"
  echo "2026-09-17T00:00:00.0000000Z $soigneur_marker failing TestSub/issue_#3568_-_ok=yes"
} > "$tmp"/logs/'1_in-host _ rootful linux.txt'

check "implausible test names are dropped, realistic ones are kept" \
  '{"id":"77","kind":"failing","tests":["TestLegit","TestSub/issue_#3568_-_ok=yes"]}' \
  "$(soigneur::logs::parse "$tmp"/logs "$lookup" 2>/dev/null)"

check "a log that matches no job is reported, not attributed at random" \
  'warning: no job matches the log of 1_in-host _ rootful linux' \
  "$(soigneur::logs::parse "$tmp"/logs "$(printf 'other\t99\n')" 2>&1 >/dev/null | tr -d "'")"

# --- the renderer --------------------------------------------------------------------------
render(){
  jq -n -f "$here"/flaky-report.jq \
    --slurpfile executions "$1" \
    --slurpfile findings "$2" \
    --arg repo "owner/name" --arg branch "main" \
    --arg since "2026-09-10T00:00:00Z" --arg now "2026-09-17T00:00:00Z" \
    --arg maxTests 25 --arg maxLinks 5 --arg runURL "" --arg docsURL "" --arg footer "" \
    --argjson notes '[]'
}

cat > "$tmp"/executions.json <<'EOF'
{"sha":"abc","id":"77","name":"in-host / rootful\n linux","conclusion":"failure","url":"https://example/1","at":"2026-09-17T00:00:00Z"}
{"sha":"def","id":"78","name":"in-host / rootful\n linux","conclusion":"success","url":"https://example/2","at":"2026-09-16T00:00:00Z"}
EOF
cat > "$tmp"/findings.json <<'EOF'
{"id":"77","kind":"failing","tests":["TestAlpha","TestAlpha/sub"]}
{"id":"77","kind":"flaky","tests":["TestBeta"]}
EOF

report="$(render "$tmp"/executions.json "$tmp"/findings.json)"

check "a parent test and its subtest count once, as one row" \
  '1' "$(printf '%s' "$report" | jq -r '[.data.tests[] | select(.test == "TestAlpha")] | length')"
check "the recovered test is counted as flaky" \
  '1' "$(printf '%s' "$report" | jq -r '.data.tests[] | select(.test == "TestBeta") | .flaky')"
check "the job name loses the newlines the API puts in it" \
  'in-host / rootful linux' "$(printf '%s' "$report" | jq -r '.data.jobs[0].job')"
check "the failure rate is per execution" \
  '1/2' "$(printf '%s' "$report" | jq -r '.data.jobs[0] | "\(.failures)/\(.executions)"')"

: > "$tmp"/empty.json
check "an empty window renders, and says so" \
  'true' \
  "$(render "$tmp"/empty.json "$tmp"/empty.json | jq -r '.markdown | contains("No test-level failure was reported")')"

# --- the validators ------------------------------------------------------------------------
check "false means false" "off" "$(soigneur::bool "false" && echo on || echo off)"
check "unset means false" "off" "$(soigneur::bool "" && echo on || echo off)"
check "true means true" "on" "$(soigneur::bool "TRUE" && echo on || echo off)"
check "a flag is not a number" "rejected" \
  "$(soigneur::number "N" "--body-file=/etc/passwd" 2>/dev/null && echo accepted || echo rejected)"
check "a traversal is not a repository" "rejected" \
  "$(soigneur::repo "R" "../../evil" 2>/dev/null && echo accepted || echo rejected)"

# --- the emitter ---------------------------------------------------------------------------
cat > "$tmp"/gotestsum.json <<'EOF'
{"Action":"fail","Test":"TestAlpha","Package":"p"}
{"Action":"fail","Test":"TestBeta","Package":"p"}
{"Action":"pass","Test":"TestBeta","Package":"p"}
EOF
emitted="$(SOIGNEUR_QUIET=true "$here"/flaky-annotate.sh "$tmp"/gotestsum.json)"

check "a test that never passed is failing" "true" \
  "$(printf '%s' "$emitted" | grep -qF "$soigneur_marker failing TestAlpha" && echo true || echo false)"
check "a test that passed on retry is flaky" "true" \
  "$(printf '%s' "$emitted" | grep -qF "$soigneur_marker flaky TestBeta" && echo true || echo false)"
check "and both are annotated for the pull request" "2" \
  "$(printf '%s' "$emitted" | grep -c '^::\(error\|warning\) title=')"

echo
if [ "$failed" == "0" ]; then
  echo "$total checks, all good."
else
  echo "$total checks, $failed failed."
  exit 1
fi
