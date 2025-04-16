# Requirements

Any `test.Case` has a `Require` property that allow enforcing specific, per-test requirements.

A `Requirement` is expected to make you `Skip` tests when the environment does not match
expectations.

A number of ready-made requirements are provided:

```go
require.Windows // a test runs only on Windows
require.Linux // a test runs only on Linux
test.Darwin // a test runs only on Darwin
test.OS(name string) // a test runs only on the OS `name`
require.Binary(name string) // a test requires the binary `name` to be in the PATH
require.Not(req Requirement) // a test runs only if the opposite of the requirement `req` is fulfilled
require.All(req ...Requirement) // a test runs only if all requirements are fulfilled
```

## Custom requirements

A requirement is a simple struct (`test.Requirement`), that must provide a `Check` function.

`Check` function signature is `func(data Data, helpers Helpers) (bool, string)`, that is expected
to return `true` if the environment is fine to run that test, or `false` if not.
The second parameter should return a meaningful message explaining what the requirement is.

For example: `found kernel version > 5.0`.

Given requirements can be negated with `require.Not(req)`, your message should describe the state of the environment
and not whether the conditions are met (or not met).

A `test.Requirement` can optionally provide custom `Setup` and `Cleanup` routines, in case you need to perform
specific operations before the test run (and cleanup something after).

`Setup/Cleanup` will only run if the requirement `Check` returns true.

Here is for example how the `require.Binary` method is implemented:

```go
package thing

func Binary(name string) *test.Requirement {
    return &test.Requirement{
        Check: func(_ test.Data, _ test.Helpers) (bool, string) {
            mess := fmt.Sprintf("executable %q has been found in PATH", name)
            ret := true
            if _, err := exec.LookPath(name); err != nil {
                ret = false
                mess = fmt.Sprintf("executable %q doesn't exist in PATH", name)
            }

            return ret, mess
        },
    }
}
```

## Gotcha

Obviously, `test.Not(otherreq)` will NOT perform any `Setup/Cleanup` provided by `otherreq`.

Ambient requirements are currently undocumented.
