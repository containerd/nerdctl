# Nerdctl testing tools

## Preamble

The integration test suite in nerdctl is meant to apply to both nerdctl and docker,
and further support additional test properties to target specific contexts (ipv6, kube).

Basic _usage_ is covered in the [testing docs](testing.md).

This here covers how to write tests, leveraging nerdctl `pkg/testutil/test`
which has been specifically developed to take care of repetitive tasks,
protect the developer against unintended side effects across tests, and generally
encourage clear testing structure with good debug-ability and a relatively simple API for
most cases.

## Using `test.Case`

Starting from scratch, the simplest, basic structure of a new test is:

```go
package main

import (
	"testing"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestMyThing(t *testing.T) {
	nerdtest.Setup()

	// Declare your test
	myTest := &test.Case{
		Description: "A first test",
		// This is going to run `nerdctl info` (or `docker info`)
		Command:     test.RunCommand("info"),
		// Verify the command exits with 0, and stdout contains the word `Kernel`
		Expected:    test.Expects(0, nil, test.Contains("Kernel")),
	}

	// Run it
	myTest.Run(t)
}
```

## Expectations

There are a handful of helpers for "expectations".

You already saw two (`test.Expects` and `test.Contains`):

First, `test.Expects(exitCode int, errors []error, outputCompare Comparator)`, which is
convenient to quickly describe what you expect overall.

`exitCode` is obvious.

`errors` is a slice of go `error`, that allows you to compare what is seen on stderr
with existing errors (for example: `errdefs.ErrNotFound`), or more generally
any string you want to match.

`outputCompare` can be either your own comparison function, or
one of the comparison helper.

Secondly, `test.Contains`, is a `Comparator`.

### Comparators

Besides `test.Contains(string)`, there are a few more:
- `test.DoesNotContain(string)`
- `test.Equals(string)`
- `test.All(comparators ...Comparator)`, which allows you to bundle together a bunch of other comparators

The following example shows how to implement your own custom `Comparator`
(this is actually the `Equals` comparator).

```go
package whatever

import (
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func MyComparator(compare string) test.Comparator {
	return func(stdout string, info string, t *testing.T) {
		t.Helper()
		assert.Assert(t, stdout == compare, info)
	}
}
```

Note that you have access to an opaque `info` string.
It contains relevant debugging information in case your comparator is going to fail,
and you should make sure it is displayed.

### Advanced expectations

You may want to have expectations that contain a certain piece of data
that is being used in the command or at other stages of your test (Setup).

For example, creating a container with a certain name, you might want to verify
that this name is then visible in the list of containers.

To achieve that, you should write your own `Expecter`, leveraging test `Data`.

Here is an example, where we are using `data.Get("sometestdata")`.

```go
package main

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestMyThing(t *testing.T) {
	nerdtest.Setup()

	// Declare your test
	myTest := &test.Case{
		Description: "A subtest with custom data, manager, and comparator",
		Data:        test.WithData("sometestdata", "blah"),
		Command:     test.RunCommand("info"),
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				ExitCode: 1,
				Errors: []error{
					errors.New("foobla"),
					errdefs.ErrNotFound,
				},
				Output: func(stdout string, info string, t *testing.T) {
					assert.Assert(t, stdout == data.Get("sometestdata"), info)
				},
			}
		},
	}

	myTest.Run(t)
}
```

## On `Data`

`Data` is provided to first allow storing mutable key-value information that pertain to the test.

While it can be provided through `WithData(key string, value string)` (or `WithConfig`, see below),
inside the testcase definition, it can also be dynamically manipulated inside `Setup`, or `Command`.

Note that `Data` additionally exposes the following functions:
- `Identifier()` which returns the unique id associated with the _current_ test (or subtest)
- `TempDir()` which returns the private, temporary directory associated with the test

... along with the `Get(key)` and `Set(key, value)` methods.

Secondly, `Data` allows defining and manipulating "configuration" data.

In the case of nerdctl here, the following configuration options are defined:
- `WithConfig(Docker, NotCompatible)` to flag a test as not compatible
- `WithConfig(Mode, Private)` will entirely isolate the test using a different
namespace, data root, nerdctl config, etc
- `WithConfig(NerdctlToml, "foo")` which allows specifying a custom config
- `WithConfig(DataRoot, "foo")` allowing to point to a custom data-root
- `WithConfig(HostsDir, "foo")` to point to a specific hosts directory
- `WithConfig(Namespace, "foo")` allows passing a specific namespace (otherwise defaults to `nerdctl-test`)

## Commands

For simple cases, `test.RunCommand(args ...string)` is the way to go.

It will execute the binary to test (nerdctl or docker), with the provided arguments,
and will by default get cwd inside the temporary directory associated with the test.

### Environment

You can attach custom environment variables for your test in the `Env` property of your
test.

These will be automatically added to the environment for your command, and also
your setup and cleanup routines (see below).

If you would like to override the environment specifically for a command, but not for
others (eg: in `Setup` or `Cleanup`), you can do so with custom commands (see below).

Note that environment as defined statically in the test will be inherited by subtests,
unless explicitly overridden.

### Working directory

By default, the working directory of the command will be set to the temporary directory
of the test.

This behavior can be overridden using custom commands.

### Custom commands

Custom commands allow you to manipulate test `Data`, override important aspects
of the command to execute (`Env`, `WorkingDir`), or otherwise give you full control
on what the command does.

You just need to implement an `Executor`:

```go
package main

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestMyThing(t *testing.T) {
	nerdtest.Setup()

	// Declare your test
	myTest := &test.Case{
		Description: "A subtest with custom data, manager, and comparator",
		Data:        test.WithData("sometestdata", "blah"),
		Command: func(data test.Data, helpers test.Helpers) test.Command {
			return helpers.Command("run", "--name", data.Identifier()+data.Get("sometestdata"))
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				ExitCode: 1,
				Errors: []error{
					errors.New("foobla"),
					errdefs.ErrNotFound,
				},
				Output: func(stdout string, info string, t *testing.T) {
					assert.Assert(t, stdout == data.Get("sometestdata"), info)
				},
			}
		},
	}

	myTest.Run(t)
}
```

### On `helpers`

Inside a custom `Executor`, `Manager`, or `Butler`, you have access to a collection of
`helpers` to simplify command execution:

- `helpers.Ensure(args ...string)` will run a command and ensure it exits succesfully
- `helpers.Fail(args ...string)` will run a command and ensure it fails
- `helpers.Anyhow(args ...string)` will run a command but does not care if it succeeds or fails
- `helpers.Capture(args ...string)` will run a command, ensure it is successful, and return the output
- `helpers.Command(args ...string)` will return a command that can then be tested against expectations
- `helpers.CustomCommand(binary string, args ...string)` will do the same for any arbitrary command (not limited to nerdctl)

## Setup and Cleanup

Tests routinely require a set of actions to be performed _before_ you can run the
command you want to test.
A setup routine will get executed before your `Command`, and have access to and can
manipulate your test `Data`.

Conversely, you very likely want to clean things up once your test is done.
While temporary directories are cleaned for you with no action needed on your part,
the app you are testing might have stateful data you may want to remove.
Note that a `Cleanup` routine will get executed twice - after your `Command` has run
its course evidently - but also, pre-emptively, before your `Setup`, so that possible leftovers from
previous runs are taken care of.

```go
package main

import (
	"errors"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/errdefs"

	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
	"github.com/containerd/nerdctl/v2/pkg/testutil/test"
)

func TestMyThing(t *testing.T) {
	nerdtest.Setup()

	// Declare your test
	myTest := &test.Case{
		Description: "A subtest with custom data, manager, and comparator",
		Data:        test.WithData("sometestdata", "blah"),
		Setup: func(data *test.Data, helpers test.Helpers){
			helpers.Ensure("volume", "create", "foo")
			helpers.Ensure("volume", "create", "bar")
		},
		Cleanup: func(data *test.Data, helpers test.Helpers){
			helpers.Anyhow("volume", "rm", "foo")
			helpers.Anyhow("volume", "rm", "bar")
		},
		Command: func(data test.Data, helpers test.Helpers) test.Command {
			return helpers.Command("run", "--name", data.Identifier()+data.Get("sometestdata"))
		},
		Expected: func(data test.Data, helpers test.Helpers) *test.Expected {
			return &test.Expected{
				ExitCode: 1,
				Errors: []error{
					errors.New("foobla"),
					errdefs.ErrNotFound,
				},
				Output: func(stdout string, info string, t *testing.T) {
					assert.Assert(t, stdout == data.Get("sometestdata"), info)
				},
			}
		},
	}

	myTest.Run(t)
}
```

## Subtests

Subtests are just regular tests, attached to the `SubTests` slice of a test.

Note that a subtest will inherit its parent `Data` and `Env`, in the state they are at
after the parent test has run its `Setup` and `Command` routines (but before `Cleanup`).
This does _not_ apply to `Identifier()` and `TempDir()`, which are unique to the sub-test.

Also note that a test does not have to have a `Command`.
This is a convenient pattern if you just need a common `Setup` for a bunch of subtests.

## Groups

A `test.Group` is just a convenient way to represent a slice of tests.

Note that unlike a `test.Case`, a group cannot define properties inherited by
subtests, nor `Setup` or `Cleanup` routines.

- if you just have a bunch of subtests you want to run, put them in a `test.Group`
- if you want to have a global setup, or otherwise set a common property first for your subtests, use a `test.Case` with `SubTests`

## Parallelism

All tests (and subtests) are assumed to be parallelizable.

You can force a specific `test.Case` to not be run in parallel though,
by setting its `NoParallel` property to `true`.

Note that if you want better isolation, it is usually better to use
`WithConfig(nerdtest.Mode, nerdtest.Private)` instead.
This will keep the test parallel (for nerdctl), but isolate it in a different context.
For Docker (which does not support namespaces), it is equivalent to passing `NoParallel: true`.
