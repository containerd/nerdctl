# Soigneur

A flaky test is a test that fails, then passes, without anything having changed. The first
problem with those is *knowing* about them: a failure that nobody looks at twice is a failure
that stays.

Soigneur keeps a **flaky test dashboard**: a single, long-lived GitHub issue holding what the CI
reported over the last week. It runs no test of its own - it watches the ones that do, and says
which of them need care. A *soigneur* is the keeper who tends the animals; the test framework
next door is [Tigron](../tigron).

Two halves, with no infrastructure behind them:

- **Emit**: at the end of a test job, [`flaky-annotate.sh`](flaky-annotate.sh) turns a
  `gotestsum` json file into one marker line per test in the job log - the tests that failed, and
  the tests that failed and then *passed on retry* - plus the matching annotations, for the
  humans reading the pull request.
- **Collect**: once a week, [`flaky-report.sh`](flaky-report.sh) lists the workflow runs of a
  branch and, for each of them, downloads the archived logs of all of its jobs in a single
  request and reads the markers back; then [`flaky-issue.sh`](flaky-issue.sh) publishes the
  aggregate to a single, long-lived issue.

No database, no test result artifact, and no server: the logs of the analyzed runs *are* the
storage. The trade-off is the window, which is limited by the retention of the logs (90 days, by
default).

## What the dashboard reports

For the analyzed window:

- which tests were reported as failing, how often, in how many different job configurations, and
  when they were last seen;
- which of those failures were **recovered on retry**, in other words: proven flaky;
- which job configurations failed, and how often - a job that fails without any test failing is
  usually an environment or a timeout problem, and that is worth knowing too.

Occurrences are counted per (commit × job configuration × outcome × top-level test), so a single
bad job execution counts once, and subtests do not inflate the ranking of their parent.

## Emitting

The two halves talk to each other through one marker line per test, written to the job log:

```
flaky-test-dashboard: failing TestFoo
flaky-test-dashboard: flaky TestBar/subtest
```

| Class | Meaning |
| --- | --- |
| `failing` | Failed, and never passed - even on retry. Flaky, or simply broken. |
| `flaky` | Failed, then passed when retried. Flaky beyond a doubt. |

The same two classes are also emitted as GitHub Actions annotations (`Failing tests` as an
error, `Flaky tests` as a warning), which is what shows up on a pull request. The collector does
not read those: annotations cost one API request per job, markers cost none, since the logs of a
whole run come in a single archive.

`flaky-annotate.sh` derives both classes from a `gotestsum` json file, so the retry information
is only there if the tests were actually retried (`--rerun-fails`):

```yaml
- name: "Run: tests"
  run: |
    gotestsum \
      --jsonfile=/tmp/tests.json \
      --rerun-fails=2 \
      --post-run-command ./mod/soigneur/flaky-annotate.sh \
      -- ./...
```

It can also be called as a plain step (`flaky-annotate.sh /tmp/tests.json`), and it writes the
two lists to `SOIGNEUR_FAILING_OUT` / `SOIGNEUR_SOIGNEUR_OUT` for callers that render their own job
summary. Projects that do not use `gotestsum` only have to print those marker lines themselves,
one per test, to be collected. The marker and the annotation titles are defined in
[`lib.sh`](lib.sh).

## Collecting, with the action

```yaml
name: flaky-test-dashboard

on:
  schedule:
    - cron: "0 7 * * 1"  # every Monday
  workflow_dispatch:

permissions:
  contents: read

jobs:
  dashboard:
    runs-on: ubuntu-latest
    permissions:
      contents: read  # fetch the action from this repository
      actions: read  # list the workflow runs, and download their logs
      issues: write  # create, and update, the dashboard issue
    steps:
      - uses: actions/checkout@v5
        with:
          persist-credentials: false
      - uses: containerd/nerdctl/mod/soigneur@main
        with:
          branch: main
          days: 7
          issue-number: "1234"  # the issue holding the dashboard
          # Optional: the workflows that actually run tests. Without it, the logs of every
          # workflow run of the window are downloaded, which only costs time.
          workflows: "test.yml,nightly.yml"
```

Open the dashboard issue by hand once, and name its number: its description is rewritten with the
latest report, and the digest is posted as a comment, so that the subscribers get notified without
the issue growing a copy of every report. Soigneur never opens an issue itself, which is the
point - there is exactly one dashboard, and it is the one you named. To start a fresh one, open it
and change the number.

### Inputs

All of them are optional. See [`action.yml`](action.yml) for the defaults.

| Input | Description |
| --- | --- |
| `token` | Token used to read the run logs and write the issue. Needs `actions: read` and `issues: write`. |
| `repository`, `branch`, `days`, `event` | What to report on. `event` defaults to `push`, the post-merge signal. |
| `workflows` | Only download the logs of these workflow files. |
| `max-runs`, `parallel` | Bound the number, and the concurrency, of the runs collected. |
| `max-tests`, `max-links` | How much detail the report carries. |
| `publish`, `comment` | Turn off the issue update, or just the digest comment (dry run). |
| `issue-number` | Which issue is *the* dashboard. Required. |
| `summary` | Whether to also write the report to the run summary. |
| `docs-url`, `footer-notes` | Project-specific pointers and caveats, rendered in the report. |

### Outputs

`report-file`, `json-file`, `digest`, and `issue-url`. The json is the aggregate the markdown was
rendered from, for projects that want to slice it differently:

```bash
jq '.tests[] | select(.flaky > 0) | .test' "$JSON_FILE"
```

## Running it locally

The collector only needs `gh` (authenticated), `jq`, `unzip`, and read access to the repository:

```bash
# The last 7 days of main, as markdown, on stdout
SOIGNEUR_REPO=containerd/nerdctl ./flaky-report.sh

# A different window or branch
SOIGNEUR_DAYS=30 SOIGNEUR_BRANCH=release/2.2 ./flaky-report.sh

# Keep (and re-read) the API responses, which makes iterating on the report almost free
SOIGNEUR_WORKDIR=/tmp/flaky ./flaky-report.sh
```

Every knob is an environment variable, documented at the top of each script.

## Cost

Collecting costs two API requests per workflow run - one for the jobs, one for the log archive -
and the archive itself, which is about 1 MB for a run of twenty jobs. A busy week of a single
branch measures at roughly 100 runs, so 200 requests and 50 MB, against the 1000 requests per
hour and per repository that the `GITHUB_TOKEN` of a workflow gets.

The per-job alternative, reading the annotations through the checks API, costs one request per
job execution instead - about 800 requests for the same week, uncomfortably close to that limit,
which is why the markers in the logs are what gets collected. The annotations remain, for the
humans.

Note that the logs are the only thing that can be read back: GitHub's job summaries are not
exposed by any API, only rendered in the web UI from signed attachments.

The downloaded logs are whole job logs, written to a temporary directory that is removed when the
report is done - unless `SOIGNEUR_WORKDIR` is set, in which case they stay there. GitHub masks the
registered secrets in the logs it archives, but a log still holds everything else the run
printed, so do not upload a workdir as an artifact.

The test names, too, come out of those logs, which means they are only as trustworthy as what the
tests printed: a test that prints a line shaped like a marker gets it collected. The collector
therefore keeps only the names that can plausibly be a Go test name, and the renderer escapes
what ends up in the tables, so that a crafted name cannot turn into a link, or into markup, in
the issue.

## License

Apache License 2.0. See [LICENSE](LICENSE).
