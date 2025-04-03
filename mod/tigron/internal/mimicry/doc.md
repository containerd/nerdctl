# [INTERNAL] [EXPERIMENTAL] Mocker

## Creating a Mock

```golang
package mymock

import "github.com/containerd/nerdctl/mod/tigron/internal/mimicry"

type InterfaceToBeMocked interface {
    SomeMethod(one string, two int) error
}

// Compile time ensure the mock does fulfill the interface

var _ InterfaceToBeMocked = &MyMock{}

type MyMock struct {
    // Embed mimicry core
	mimicry.Core
}

// Describe function parameters and return values.

type (
    MyMockSomeMethodIn struct {
        one string
        two int
    }

    MyMockSomeMethodOut = error
)

// Satisfy the interface + wire-in the handler mechanism

func (m *MyMock) SomeMethod(one string, two int) error {
    if handler := m.Retrieve(); handler != nil {
        return handler.(mimicry.Function[MyMockSomeMethodIn, MyMockSomeMethodOut])(MyMockSomeMethodIn{
            one: one,
            two: two,
        })
    }

    return nil
}
```


## Using a Mock

Simplest way to use it is to inspect calls after the fact:

```golang
package mymock

import "testing"

func functionYouWantToTest(o InterfaceToBeMocked, i int) {
    o.SomeMethod("lala", i)
}

func TestOne(t *testing.T) {
    mocky := &MyMock{}

    functionYouWantToTest(mocky, 42)
    functionYouWantToTest(mocky, 123)

    // Now you can inspect the calls log for the function SomeMethod.

    report := mocky.Report(InterfaceToBeMocked.SomeMethod)
    t.Log("Number of times it was called:", len(report))
    t.Log("Inspecting the last call:")
    t.Log(mimicry.PrintCall(report[len(report)-1]))
}
```

## Using handlers

It is also possible to implement handlers to intercept the calls at the time they are made.

```golang
package main_test

import "testing"

func functionYouWantToTest(o InterfaceToBeMocked, i int) {
    o.SomeMethod("lala", i)
}

func TestTwo(t *testing.T) {
    mocky := &MyMock{}

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
