# Flaky tests

A flaky test is a test that fails, then passes, without anything having changed.

nerdctl runs its integration suite against a dozen different environments (rootful, rootless,
ipv6, arm64, older ubuntu, older containerd, almalinux, Docker, ...), on ephemeral runners, with
real daemons, real networking, and real images. Some tests do occasionally fail there without
anybody having broken anything, and the first problem with those is *knowing* about them: a
failure that nobody looks at twice is a failure that stays.

This is what the **flaky test dashboard** is for.

## The dashboard

The dashboard is a single, long-lived GitHub issue,
[containerd/nerdctl#5202](https://github.com/containerd/nerdctl/issues/5202), whose description is
rewritten every Monday by the
[`flaky-test-dashboard`](../../.github/workflows/flaky-test-dashboard.yml) workflow. Every week it also
posts a one-line digest as a comment, so that the subscribers get notified without the issue
growing a copy of every report.

It reports, for the last seven days of `main`:

- which tests were reported as failing, how often, and in how many different job configurations;
- which of those failures were *recovered on retry*, in other words: proven flaky;
- which job configurations failed, and how often - a job that fails without any test failing is
  usually an environment or a timeout problem, and that is worth knowing too.

Note that:

- Only the integration suites report per-test data. The unit tests, and the Windows and FreeBSD
  jobs, do not run the reporter (see the `--post-run-command` in
  [`hack/test-integration.sh`](../../hack/test-integration.sh)).
- A test that is reported as failing is not necessarily flaky: it may simply be broken. Only the
  "recovered on retry" column proves flakiness by itself.
- The window cannot be extended arbitrarily: the data lives in the logs of the analyzed runs,
  which GitHub retains for 90 days by default.

## How it works

There is no test result database, no artifact to download, and no server to operate. The CI
already annotates its own test jobs, and the dashboard is just a weekly aggregation of those
annotations:

1. [`hack/test-integration.sh`](../../hack/test-integration.sh) runs the suite with
   `gotestsum --jsonfile`, and, for the flaky suite, with `--rerun-fails`.
2. [`hack/github/gotestsum-reporter.sh`](../../hack/github/gotestsum-reporter.sh) reads that json
   file when the run is over, and hands it to
   [`flaky-annotate.sh`](../../mod/soigneur/flaky-annotate.sh), which writes one marker
   line per test to the job log, in two classes:
   - `flaky-test-dashboard: failing TestFoo`: failed, and never passed, even on retry;
   - `flaky-test-dashboard: flaky TestFoo`: failed, then passed on retry.

   The same two classes are also emitted as annotations (`Failing tests`, `Flaky tests`), which
   is what shows up on a pull request.
3. [`flaky-report.sh`](../../mod/soigneur/flaky-report.sh) lists the workflow runs of the
   branch and, for each of them, downloads the archived logs of all of its jobs in a single
   request, reads the markers back, and renders the aggregate as markdown.
4. [`flaky-issue.sh`](../../mod/soigneur/flaky-issue.sh) publishes the result to the
   dashboard issue.

Steps 2 to 4 are not nerdctl-specific, and live in [Soigneur](../../mod/soigneur), a reusable
action - a *soigneur* is the keeper who tends the animals, and this one keeps an eye on the test
suite. [`.github/workflows/flaky-test-dashboard.yml`](../../.github/workflows/flaky-test-dashboard.yml)
calls it with the nerdctl-specific bits (the branch, the window, the link to this document). Its
[README](../../mod/soigneur/README.md) covers the inputs, the outputs, and the marker
contract - the marker lines are an API between the two halves, so do not change them on one side
only. Note that only the runs of the `push` event are reported on, which is the post-merge
signal: a failure on a pull request is usually the pull request's own doing.

## Running the collector locally

The collector only needs `gh` (authenticated), `jq`, and read access to the repository:

```bash
export SOIGNEUR_REPO=containerd/nerdctl

# The last 7 days of main, as markdown, on stdout
./mod/soigneur/flaky-report.sh

# A different window, or branch
SOIGNEUR_DAYS=30 SOIGNEUR_BRANCH=release/2.2 ./mod/soigneur/flaky-report.sh

# Keep (and re-read) the API responses, which makes iterating on the report almost free
SOIGNEUR_WORKDIR=/tmp/flaky ./mod/soigneur/flaky-report.sh

# Get the aggregate as json instead, to slice it differently
SOIGNEUR_JSON_OUT=/tmp/flaky.json ./mod/soigneur/flaky-report.sh > /dev/null
jq '.tests[] | select(.flaky > 0) | .test' /tmp/flaky.json
```

Collecting a week costs about 200 API requests and downloads some 50 MB of logs, which is why
the workflow runs weekly, on a quiet hour, and only for the workflows that do run tests.
`SOIGNEUR_DAYS`, `SOIGNEUR_MAX_RUNS`, and the other knobs are documented at the top of the script;
`SOIGNEUR_WORKDIR` keeps the downloaded logs around, which makes a second look free.

The dashboard itself can be refreshed at any time by dispatching the workflow manually
(`Actions` > `flaky-test-dashboard` > `Run workflow`), which also accepts a window and a branch, and
can be told not to touch the issue at all (`publish: false`), in which case the report is only
written to the run summary.

## Working on a flaky test

To reproduce, run the test in a loop, in the environment the dashboard points at:

```bash
go test ./cmd/nerdctl/container -run 'TestRunSomething' -count 10 -p 1
```

If it does not fail, try it under load (`-parallel`, or simply another suite running at the same
time), as most flakiness in this project comes from timing and from resource contention.

A test that is known to be flaky, and that cannot be fixed right away, should be marked as such
rather than left to fail at random:

```go
testCase.Require = nerdtest.IsFlaky("https://github.com/containerd/nerdctl/issues/1234")
```

Flaky tests are then only run by the dedicated `-test.only-flaky=true` pass, which retries them
(`--rerun-fails`), and which the
[`[flaky, see #3988]`](../../.github/workflows/workflow-flaky.yml) workflow may skip entirely.
This keeps the signal of the main suites clean - at the price of no longer really testing what
those tests cover, so please do link an issue, and do come back to it.

See also [tools.md](tools.md) for the test framework itself, and
[README.md](README.md) for how to run the suites.
