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

# Collects the test failures that the CI reported over a period of time, and renders them as a
# markdown "flaky test dashboard" on stdout.
#
# The data source is the archived log of every workflow run of the window: one request per run
# returns a zip holding the log of each of its jobs, from which the marker lines that
# flaky-annotate.sh wrote (see lib.sh) are read back. There is no database, no test result
# artifact, and no server involved: the logs of the analyzed runs *are* the storage, which also
# means that the window is limited by the retention of the logs (90 days, by default).
# See README.md.
#
# Usage:
#   gh auth login                  # or export GH_TOKEN
#   ./flaky-report.sh              # markdown to stdout
#
# Environment:
#   SOIGNEUR_REPO        repository to analyze (default: $GITHUB_REPOSITORY)
#   SOIGNEUR_BRANCH      branch to analyze (default: main)
#   SOIGNEUR_DAYS        size of the window, in days (default: 7)
#   SOIGNEUR_EVENT       only analyze the runs of that event, empty for all (default: push)
#   SOIGNEUR_WORKFLOWS   only analyze the runs of these workflow files, as a comma-separated list
#                        of file names, empty for all (default: empty)
#   SOIGNEUR_MAX_RUNS    maximum number of runs to analyze (default: 200)
#   SOIGNEUR_MAX_TESTS   maximum number of tests to detail (default: 25)
#   SOIGNEUR_MAX_LINKS   maximum number of links per test or job (default: 5)
#   SOIGNEUR_PARALLEL    number of runs to collect concurrently (default: 4)
#   SOIGNEUR_API_TIMEOUT per-request timeout, in seconds (default: 300)
#   SOIGNEUR_JSON_OUT    if set, the raw aggregated data is written to that file
#   SOIGNEUR_DIGEST_OUT  if set, a one-line summary is written to that file
#   SOIGNEUR_RUN_URL     link back to the run that generated the report (default: none)
#   SOIGNEUR_DOCS_URL    where the project documents its flaky tests, linked from the report
#   SOIGNEUR_FOOTER      extra markdown for the "How this is collected" section, for the caveats
#                        that are specific to the project (which suites do report per-test data...)
#   SOIGNEUR_WORKDIR     if set, the downloaded logs and the intermediate results are kept in (and
#                        re-read from) that directory, which makes iterating on the report cheap

set -o errexit -o errtrace -o functrace -o nounset -o pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]:-$PWD}")" 2>/dev/null 1>&2 && pwd)"
readonly root

# shellcheck source-path=SCRIPTDIR
. "$root"/lib.sh

: "${SOIGNEUR_REPO:=${GITHUB_REPOSITORY:-}}"
: "${SOIGNEUR_BRANCH:=main}"
: "${SOIGNEUR_DAYS:=7}"
: "${SOIGNEUR_EVENT:=push}"
: "${SOIGNEUR_WORKFLOWS:=}"
: "${SOIGNEUR_MAX_RUNS:=200}"
: "${SOIGNEUR_MAX_TESTS:=25}"
: "${SOIGNEUR_MAX_LINKS:=5}"
: "${SOIGNEUR_PARALLEL:=4}"
: "${SOIGNEUR_API_TIMEOUT:=300}"
: "${SOIGNEUR_JSON_OUT:=}"
: "${SOIGNEUR_DIGEST_OUT:=}"
: "${SOIGNEUR_RUN_URL:=}"
: "${SOIGNEUR_DOCS_URL:=}"
: "${SOIGNEUR_FOOTER:=}"
: "${SOIGNEUR_WORKDIR:=}"
export SOIGNEUR_REPO SOIGNEUR_API_TIMEOUT

[ "$SOIGNEUR_REPO" != "" ] || {
  echo "error: no repository to analyze: set SOIGNEUR_REPO or GITHUB_REPOSITORY" >&2
  exit 1
}

soigneur::repo "SOIGNEUR_REPO" "$SOIGNEUR_REPO"
for knob in SOIGNEUR_DAYS SOIGNEUR_MAX_RUNS SOIGNEUR_MAX_TESTS SOIGNEUR_MAX_LINKS SOIGNEUR_PARALLEL SOIGNEUR_API_TIMEOUT; do
  soigneur::number "$knob" "${!knob}"
done

# A single hung request would otherwise hang the whole report: `gh` has no request timeout.
api=(gh api --header "Accept: application/vnd.github+json")
! command -v timeout > /dev/null || api=(timeout "$SOIGNEUR_API_TIMEOUT" "${api[@]}")
readonly api

# Retries on the failures that are worth retrying. A 4xx is an answer, not a hiccup: a run whose
# logs have expired, or that was deleted, is expected and must not cost three attempts.
soigneur::api(){
  local attempt=1
  local err

  err="$(mktemp)"
  while true; do
    if "${api[@]}" "$@" 2> "$err" < /dev/null; then
      rm -f "$err"
      return 0
    fi
    if grep -q "HTTP 4" "$err"; then
      cat "$err" >&2
      rm -f "$err"
      return 1
    fi
    [ "$attempt" -lt 3 ] || {
      cat "$err" >&2
      echo "error: giving up on: gh api $*" >&2
      rm -f "$err"
      return 1
    }
    echo "warning: retrying: gh api $*" >&2
    sleep "$((attempt * 5))"
    attempt="$((attempt + 1))"
  done
}

# Lists the completed workflow runs of the window, as one json object per line.
soigneur::runs(){
  local since="$1"
  local query="repos/$SOIGNEUR_REPO/actions/runs?status=completed&per_page=100"
  query="$query&branch=$SOIGNEUR_BRANCH&created=>=$since"
  [ "$SOIGNEUR_EVENT" == "" ] || query="$query&event=$SOIGNEUR_EVENT"

  soigneur::api --paginate "$query" \
    | jq -c --arg wanted "$SOIGNEUR_WORKFLOWS" '
        ($wanted | split(",") | map(sub("^ +"; "") | sub(" +$"; "")) | map(select(length > 0))) as $wanted
        | .workflow_runs[]
        | {
            id: (.id | tostring),
            attempts: (.run_attempt // 1),
            workflow: (.path | sub("^\\.github/workflows/"; "")),
            at: .created_at
          }
        | select(($wanted | length) == 0 or ($wanted | index(.workflow)))'
}

# A job log holds whatever the tests printed, so a test that prints a line shaped like a marker
# gets that line collected. Since the collected names are rendered into a GitHub issue, only the
# ones that can plausibly be a Go test name are kept: no backtick to break out of the code span
# they are rendered in, no bracket to turn them into a link, and a bounded length.
soigneur::plausible(){
  awk -F'\t' '
    NF != 2 { next }
    length($2) <= 200 && $2 ~ /^[A-Za-z0-9_.\/#:@+=,()~^-]+$/ { print; next }
    { printf "warning: ignoring an implausible test name: %s\n", substr($2, 1, 60) > "/dev/stderr" }
  '
}

# Reads the marker lines out of the job logs of one extracted archive, and prints one json object
# per (job, class) found. Takes a job name -> job id lookup, as tsv.
soigneur::logs::parse(){
  local dir="$1"
  local lookup="$2"
  local -A jobid=()
  local file name key id markers

  while IFS=$'\t' read -r key id; do
    [ "$key" == "" ] || jobid["$key"]="$id"
  done <<< "$lookup"

  # Only the top-level "<index>_<job name>.txt" entries are whole job logs: the per-step files
  # live in a directory named after the job, and would count twice.
  for file in "$dir"/*.txt; do
    [ -e "$file" ] || continue

    # GitHub sanitizes the job name for the file name (the slashes and the colons are dropped, the
    # newlines that a multi-line `name:` leaves behind are collapsed), so the two are matched on
    # their letters and digits only.
    name="$(basename "$file" .txt)"
    key="$(printf '%s' "${name#*_}" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9')"
    id="${jobid[$key]:-}"
    [ "$id" != "" ] || {
      echo "warning: no job matches the log of '$name'" >&2
      continue
    }

    markers="$(tr -d '\r' < "$file" \
      | { grep -oE "$soigneur_marker (failing|flaky) [^[:space:]]+" || true; } \
      | awk '{ print $2 "\t" $3 }' \
      | sort -u)"

    # The logs that predate the markers only hold the block that flaky-annotate.sh prints for
    # humans. Reading it as a fallback gives the dashboard the history it would otherwise have to
    # wait a full window for. Every log line starts with a timestamp, hence the first field.
    [ "$markers" == "" ] && markers="$(tr -d '\r' < "$file" \
      | awk '
          { sub(/^[^ ]+ /, "") }
          /^=== Failing tests ===$/ { kind = "failing"; next }
          /^=== Flaky tests/ { kind = "flaky"; next }
          /^====/ { kind = ""; next }
          kind != "" && $1 != "" { print kind "\t" $1 }
        ' \
      | sort -u)" || true

    printf "%s" "$markers" | soigneur::plausible | jq -R -s -c --arg id "$id" '
      split("\n")
      | map(select(length > 0) | split("\t"))
      | group_by(.[0])
      | map({id: $id, kind: .[0][0], tests: map(.[1])})[]'
  done
}

# Collects one run: its job executions, and the markers of every attempt's logs.
soigneur::run::collect(){
  local dir="$1"
  local id="$2"
  local attempts="$3"
  local jobs zip extracted lookup n

  [ ! -e "$dir/$id.done" ] || return 0

  jobs="$(soigneur::api "repos/$SOIGNEUR_REPO/actions/runs/$id/jobs?per_page=100&filter=all")" || {
    echo "$id" >> "$dir/$id.failed"
    return 0
  }

  jq -c '
    .jobs[]
    | {
        sha: .head_sha,
        id: (.id | tostring),
        name: .name,
        conclusion: .conclusion,
        url: .html_url,
        at: (.completed_at // .started_at)
      }' <<< "$jobs" > "$dir/$id.executions"

  : > "$dir/$id.findings"
  for ((n = 1; n <= attempts; n++)); do
    zip="$dir/$id-$n.zip"
    if [ ! -s "$zip" ]; then
      soigneur::api "repos/$SOIGNEUR_REPO/actions/runs/$id/attempts/$n/logs" > "$zip" || {
        echo "warning: no logs for run $id, attempt $n" >&2
        rm -f "$zip"
        echo "$id/$n" >> "$dir/$id.nologs"
        continue
      }
    fi

    extracted="$(mktemp -d)"
    unzip -o -q "$zip" -d "$extracted" || {
      echo "warning: cannot extract the logs of run $id, attempt $n" >&2
      rm -rf "$extracted"
      echo "$id/$n" >> "$dir/$id.nologs"
      continue
    }

    lookup="$(jq -r --argjson n "$n" '
      .jobs[]
      | select(.run_attempt == $n)
      | [(.name | ascii_downcase | gsub("[^a-z0-9]"; "")), (.id | tostring)]
      | @tsv' <<< "$jobs")"

    soigneur::logs::parse "$extracted" "$lookup" >> "$dir/$id.findings"
    rm -rf "$extracted"
  done

  touch "$dir/$id.done"
}

soigneur::main(){
  local tmp since now runs failed nologs
  local notes=()

  if [ "$SOIGNEUR_WORKDIR" != "" ]; then
    tmp="$SOIGNEUR_WORKDIR"
    mkdir -p "$tmp"
  else
    tmp="$(mktemp -d)"
    # shellcheck disable=SC2064
    trap "rm -rf '$tmp'" EXIT
  fi
  mkdir -p "$tmp"/runs

  now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  # BSD date does not know about `-d`.
  since="$(date -u -d "$SOIGNEUR_DAYS days ago" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null \
    || date -u -v-"$SOIGNEUR_DAYS"d +%Y-%m-%dT%H:%M:%SZ)"

  echo "Collecting $SOIGNEUR_REPO $SOIGNEUR_BRANCH from $since to $now" >&2

  [ -s "$tmp"/runs.json ] || soigneur::runs "$since" \
    | awk -v max="$SOIGNEUR_MAX_RUNS" 'NR <= max' > "$tmp"/runs.json
  runs="$(wc -l < "$tmp"/runs.json)"
  echo "Workflow runs: $runs" >&2
  [ "$runs" -lt "$SOIGNEUR_MAX_RUNS" ] || notes+=(
    "Only the $runs most recent workflow runs of the window were analyzed (\`SOIGNEUR_MAX_RUNS\`)."
  )

  # One request for the jobs, and one archive download, per run.
  jq -r '[.id, .attempts] | @tsv' < "$tmp"/runs.json \
    | tr '\t' '\n' \
    | xargs -r -P "$SOIGNEUR_PARALLEL" -n 2 "$root"/flaky-report.sh --collect-run "$tmp"/runs

  find "$tmp"/runs -name '*.executions' -exec cat '{}' + > "$tmp"/executions.json
  find "$tmp"/runs -name '*.findings' -exec cat '{}' + > "$tmp"/findings.json
  echo "Job executions: $(wc -l < "$tmp"/executions.json)" >&2

  failed="$(find "$tmp"/runs -name '*.failed' | wc -l)"
  [ "$failed" == "0" ] || notes+=(
    "$failed of the $runs workflow runs could not be read: this report is incomplete."
  )
  nologs="$(find "$tmp"/runs -name '*.nologs' | wc -l)"
  [ "$nologs" == "0" ] || notes+=(
    "The logs of $nologs of the $runs workflow runs are gone: their test failures are missing here."
  )

  jq -n -f "$root"/flaky-report.jq \
    --slurpfile executions "$tmp"/executions.json \
    --slurpfile findings "$tmp"/findings.json \
    --arg repo "$SOIGNEUR_REPO" \
    --arg branch "$SOIGNEUR_BRANCH" \
    --arg since "$since" \
    --arg now "$now" \
    --arg maxTests "$SOIGNEUR_MAX_TESTS" \
    --arg maxLinks "$SOIGNEUR_MAX_LINKS" \
    --arg runURL "$SOIGNEUR_RUN_URL" \
    --arg docsURL "$SOIGNEUR_DOCS_URL" \
    --arg footer "$SOIGNEUR_FOOTER" \
    --argjson notes "$(printf '%s\n' "" "${notes[@]:-}" | jq -R -s 'split("\n") | map(select(length > 0))')" \
    > "$tmp"/report.json

  [ "$SOIGNEUR_JSON_OUT" == "" ] || jq '.data' < "$tmp"/report.json > "$SOIGNEUR_JSON_OUT"
  [ "$SOIGNEUR_DIGEST_OUT" == "" ] || jq -r '.digest' < "$tmp"/report.json > "$SOIGNEUR_DIGEST_OUT"
  jq -r '.markdown' < "$tmp"/report.json
}

case "${1:-}" in
  --collect-run)
    # Invoked as a subprocess, one per run: see SOIGNEUR_PARALLEL.
    soigneur::run::collect "$2" "$3" "$4"
    ;;
  "")
    soigneur::main
    ;;
  *)
    echo "error: unknown argument: $1" >&2
    exit 1
    ;;
esac
