# Expectations

Attaching expectations to a test case is how the developer can express conditions on exit code, stdout, or stderr,
to be verified for the test to pass.

The simplest way to do that is to use the helper `test.Expects(exitCode int, errors []error, outputCompare test.Comparator)`.

```go
package main

import (
    "testing"

    "github.com/containerd/nerdctl/mod/tigron/test"
)

func TestMyThing(t *testing.T) {
    // Declare your test
    myTest := &test.Case{}

    // Attach a command to run
    myTest.Command = test.Custom("ls")

    // Set your expectations
    myTest.Expected = test.Expects(expect.ExitCodeSuccess, nil, nil)

    // Run it
    myTest.Run(t)
}
```

### Exit status expectations

The first parameter, `exitCode` should be set to one of the provided `expect.ExitCodeXXX` constants:
- `expect.ExitCodeSuccess`: validates that the command ran and exited successfully
- `expect.ExitCodeTimeout`: validates that the command did time out
- `expect.ExitCodeSignaled`: validates that the command received a signal
- `expect.ExitCodeGenericFail`: validates that the command failed (failed to start, or returned a non-zero exit code)
- `expect.ExitCodeNoCheck`: does not enforce any verification at all on the command

... you may also pass explicit exit codes directly (> 0) if you want to precisely match them.

### Stderr expectations with []error

To validate that stderr contain specific information, you can pass a slice of `error` as `test.Expects`
second parameter.

The command output on stderr is then verified to contain all stringified errors.

### Stdout expectations with Comparators

The last parameter of `test.Expects` accepts a `test.Comparator`, which allows testing the content of the command
output on `stdout`.

The following ready-made `test.Comparator` generators are provided:
- `expect.Contains(string, ...string)`: verifies that stdout does contain the provided parameters
- `expect.DoesNotContain(string, ...string)`: verifies that stdout does not contain any of the passed parameters
- `expect.Equals(string)`: strict equality
- `expect.Match(*regexp.Regexp)`: regexp matching
- `expect.All(comparators ...Comparator)`: allows to bundle together a bunch of other comparators
- `expect.JSON[T any](obj T, verifier func(T, string, tig.T))`: allows to verify the output is valid JSON and optionally
pass `verifier(T, string, tig.T)` extra validation

### A complete example

```go
package main

import (
    "testing"
    "errors"

    "github.com/containerd/nerdctl/mod/tigron/tig"
    "github.com/containerd/nerdctl/mod/tigron/test"
    "github.com/containerd/nerdctl/mod/tigron/expect"
)

type Thing struct {
    Name string
}

func TestMyThing(t *testing.T) {
    // Declare your test
    myTest := &test.Case{}

    // Attach a command to run
    myTest.Command = test.Custom("bash", "-c", "--", ">&2 echo thing; echo '{\"Name\": \"out\"}'; exit 42;")

    // Set your expectations
    myTest.Expected = test.Expects(
        expect.ExitCodeGenericFail,
        []error{errors.New("thing")},
        expect.All(
            expect.Contains("out"),
            expect.DoesNotContain("something"),
            expect.JSON(&Thing{}, func(obj *Thing, info string, t tig.T) {
                assert.Equal(t, obj.Name, "something", info)
            }),
        ),
    )

    // Run it
    myTest.Run(t)
}
```

### Custom stdout comparators

If you need to implement more advanced verifications on stdout that the ready-made comparators can't do,
you can implement your own custom `test.Comparator`.

For example:

```go
package whatever

import (
    "testing"

    "gotest.tools/v3/assert"

    "github.com/containerd/nerdctl/mod/tigron/tig"
    "github.com/containerd/nerdctl/mod/tigron/test"
)

func TestMyThing(t *testing.T) {
    // Declare your test
    myTest := &test.Case{}

    // Attach a command to run
    myTest.Command = test.Custom("ls")

    // Set your expectations
    myTest.Expected = test.Expects(0, nil, func(stdout, info string, t tig.T){
        t.Helper()
        // Bla bla, do whatever advanced stuff and some asserts
    })

    // Run it
    myTest.Run(t)
}

// You can of course generalize your comparator into a generator if it is going to be useful repeatedly

func MyComparatorGenerator(param1, param2 any) test.Comparator {
    return func(stdout, info string, t tig.T) {
        t.Helper()
        // Do your thing...
        // ...
    }
}

```

You can now pass along `MyComparator(comparisonString)` as the third parameter of `test.Expects`, or compose it with
other comparators using `expect.All(MyComparator(comparisonString), OtherComparator(somethingElse))`

Note that you have access to an opaque `info` string, that provides a brief formatted header message that assert
will use in case of failure to provide context on the error.
You may of course ignore it and write your own message.

### Advanced expectations

You may want to have expectations that contain a certain piece of data that is being used in the command or at
other stages of your test (like `Setup` for example).

To achieve that, you should write your own `test.Manager` instead of using the helper `test.Expects`.

A manager is a simple function which only role is to return a `test.Expected` struct.
The `test.Manager` signature makes available `test.Data` and `test.Helpers` to you.

Here is an example, where we are using `data.Labels().Get("sometestdata")`.

```go
package main

import (
    "errors"
    "testing"

    "gotest.tools/v3/assert"

    "github.com/containerd/nerdctl/mod/tigron/test"
)

func TestMyThing(t *testing.T) {
    // Declare your test
    myTest := &test.Case{}

    myTest.Setup = func(data test.Data, helpers test.Helpers){
        // Do things...
        // ...
        // Save this for later
        data.Labels().Set("something", "lalala")
    }

    // Attach a command to run
    myTest.Command = test.Custom("somecommand")

    // Set your fully custom expectations
    myTest.Expected = func(data test.Data, helpers test.Helpers) *test.Expected {
        // With a custom Manager you have access to both the test.Data and test.Helpers to perform more
        // refined verifications.
        return &test.Expected{
            ExitCode: 1,
            Errors: []error{
                errors.New("foobla"),
            },
            Output: func(stdout, info string, t tig.T) {
                t.Helper()

                // Retrieve the data that was set during the Setup phase.
                assert.Assert(t, stdout == data.Labels().Get("sometestdata"), info)
            },
        }
    }

    // Run it
    myTest.Run(t)
}
```
