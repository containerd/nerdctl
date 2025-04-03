# [INTERNAL] [EXPERIMENTAL] Mimicry

## Creating a Mock

```golang
package mymock

import "github.com/containerd/nerdctl/mod/tigron/internal/mimicry"

// Let's assume we want to mock the following, likely defined somewhere else
// type InterfaceToBeMocked interface {
//    SomeMethod(one string, two int) error
// }

// Compile time ensure the mock does fulfill the interface
var _ InterfaceToBeMocked = &MyMock{}

type MyMock struct {
    // Embed mimicry core
	mimicry.Core
}

// First, describe function parameters and return values.
type (
    MyMockSomeMethodIn struct {
        one string
        two int
    }

    MyMockSomeMethodOut = error
)

// Satisfy the interface + wire-in the handler mechanism

func (m *MyMock) SomeMethod(one string, two int) error {
	// Call mimicry method Retrieve that will record the call, and return a custom handler if one is defined
    if handler := m.Retrieve(); handler != nil {
		// Call the optional handler if there is one.
        return handler.(mimicry.Function[MyMockSomeMethodIn, MyMockSomeMethodOut])(MyMockSomeMethodIn{
            one: one,
            two: two,
        })
    }

    return nil
}
```


## Using a Mock

For consumers, the simplest way to use the mock is to inspect calls after the fact:

```golang
package mymock

import "testing"

// This is the code you want to test, that does depend on the interface we are mocking.
// func functionYouWantToTest(o InterfaceToBeMocked, i int) {
//    o.SomeMethod("lala", i)
// }

func TestOne(t *testing.T) {
	// Create the mock from above
    mocky := &MyMock{}

	// Call the function you want to test
    functionYouWantToTest(mocky, 42)
    functionYouWantToTest(mocky, 123)

    // Now you can inspect the calls log for that function.
    report := mocky.Report(InterfaceToBeMocked.SomeMethod)
    t.Log("Number of times it was called:", len(report))
    t.Log("Inspecting the last call:")
    t.Log(mimicry.PrintCall(report[len(report)-1]))
}
```

## Using handlers

Implementing handlers allows active interception of the calls for more elaborate scenarios.

```golang
package main_test

import "testing"

// The method you want to test against the mock
// func functionYouWantToTest(o InterfaceToBeMocked, i int) {
//    o.SomeMethod("lala", i)
// }

func TestTwo(t *testing.T) {
	// Create the base mock
    mocky := &MyMock{}

	// Declare a custom handler for the method `SomeMethod`
    mocky.Register(InterfaceToBeMocked.SomeMethod, func(in MyMockSomeMethodIn) MyMockSomeMethodOut {
        t.Log("Got parameters", in)

        // We want to fail on that
        if in.two == 42 {
            // Print out the callstack
            report := mocky.Report(InterfaceToBeMocked.SomeMethod)
            t.Log(mimicry.PrintCall(report[len(report)-1]))
            t.Error("We do not want to ever receive 42. Inspect trace above.")
        }else{
            t.Log("all fine - we did not see 42")
        }

        return nil
    })

    functionYouWantToTest(mocky, 123)
    functionYouWantToTest(mocky, 42)
}
```
