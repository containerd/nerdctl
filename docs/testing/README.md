# Testing nerdctl

This document covers basic usage of nerdctl testing tasks, and generic recommendations
and principles about writing tests.

For more comprehensive information about nerdctl test tools, see [tools.md](tools.md).

## Code, fix, lint, rinse, repeat

```
# Do hack

# When done:
# This will ensure proper import formatting, modules, and basic formatting of your code
make fix

# And this will run all linters and report if any modification is required
make lint
```

Note that both these tasks will work on any OS, including macOS.

Also note that `make lint` does call on subtask `make lint-commits` which is going to lint commits for the range
`main..HEAD` by default (this is fine for most development flows that expect to be merged in main).

If you are targeting a specific branch that is not `main` (for example working against `release/1.7`),
you likely want to explicitly set the commit range to that instead.

eg:
`LINT_COMMIT_RANGE=target_branch..HEAD make lint-commits`
or
`LINT_COMMIT_RANGE=target_branch..HEAD make lint`

## Unit testing

```
# This will run basic unit testing
make test
```

This task must be run on a supported OS (linux, windows, or freebsd).

## Integration testing

### TL;DR

Be sure to first `make && sudo make install`

```bash
# Test all with nerdctl (rootless mode, if running go as a non-root user)
go test -p 1 ./cmd/nerdctl/...

# Test all with nerdctl rootful
go test -p 1 -exec sudo ./cmd/nerdctl/...

# Test all with docker
go test -p 1 ./cmd/nerdctl/... -args -test.target=docker

# Test just the tests(s) which names match TestVolume.*
go test -p 1 ./cmd/nerdctl/... -run "TestVolume.*"
# Or alternatively, just test the subpackage
go test ./cmd/nerdctl/volume
```

### About parallelization

By default, when `go test ./foo/...` finds subpackages, it does create _a separate test binary
per sub-package_, and execute them _in parallel_.
This effectively will make distinct tests in different subpackages to be executed in
parallel, regardless of whether they called `t.Parallel` or not.

The `-p 1` flag does inhibit this behavior, and forces go to run each sub-package
sequentially.

Note that this is different from the `--parallel` flag, which controls the amount of
parallelization that a single go test binary will use when faced with tests that do
explicitly allow it (with a call to `t.Parallel()`).

### Or test in a container

```bash
docker build -t test-integration --target test-integration .
docker run -t --rm --privileged test-integration
```

### Principles

#### Tests should be parallelized (with best effort)

##### General case

It should be possible to parallelize all tests - as such, please make sure you:
- name all resources your test is manipulating after the test identifier (`testutil.Identifier(t)`)
to guarantee your test will not interact with other tests
- do NOT use `os.Setenv` - instead, add into `base.Env`
- use `t.Parallel()` at the beginning of your test (and subtests as well of course)
- in the very exceptional case where your test for some reason can NOT be parallelized, be sure to mark it explicitly as such
with a comment explaining why

##### For "blanket" destructive operations

If you are going to use blanket destructive operations (like `prune`), please:
- use a dedicated namespace: instead of calling `testutil.Base`, call `testutil.BaseWithNamespace` 
and be sure that your namespace is named after the test id
- remove the namespace in your test `Cleanup`
- since docker does not support namespaces, be sure to:
  - only enable `Parallel` if the target is NOT docker: `	if testutil.GetTarget() != testutil.Docker { t.Parallel() }`
  - double check that what you do in the default namespace is safe

#### Clean-up after (and before) yourself

- do NOT use `defer`, use `t.Cleanup`
- do NOT test the result of commands doing the cleanup - it is fine if they fail, 
and they are not test failure per-se - they are here to garbage collect
- you should call your cleanup routine BEFORE doing anything, in case there is any 
leftovers from previous runs, typically:
```
tearDown := func(){
    // Do some cleanup
}

tearDown()
t.Cleanup(tearDown)
```

#### Test what you are testing, and not something else

You should only test atomically.

If you are testing `nerdctl volume create`, make sure that your test will not fail
because of changes in `nerdctl volume inspect`.

That obviously means there are certain things you cannot test "yet".
Just put the right test in the right place with this simple rule of thumb:
if your test requires another nerdctl command to validate the result, then it does
not belong here. Instead, it should be a test for that other command.

Of course, this is not perfect, and changes in `create` may now fail in `inspect` tests 
while `create` could be faulty, but it does beat the alternative, because of this principle:
it is easier to walk *backwards* from a failure.