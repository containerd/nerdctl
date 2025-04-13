/*
   Copyright The containerd Authors.

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

//revive:disable:add-constant
package com_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/containerd/nerdctl/mod/tigron/internal/com"
)

func ExampleCommand() {
	cmd := com.Command{
		Binary: "printf",
		Args:   []string{"hello world"},
	}

	err := cmd.Run(context.Background())
	if err != nil {
		fmt.Println("Run err:", err)

		return
	}

	exec, err := cmd.Wait()
	if err != nil {
		fmt.Println("Wait err:", err)

		return
	}

	fmt.Println("Exit code:", exec.ExitCode)
	fmt.Println("Stdout:")
	fmt.Println(exec.Stdout)
	fmt.Println("Stderr:")
	fmt.Println(exec.Stderr)

	// Output:
	// Exit code: 0
	// Stdout:
	// hello world
	// Stderr:
	//
}

func ExampleCommand_Signal() {
	cmd := com.Command{
		Binary:  "sleep",
		Args:    []string{"3600"},
		Timeout: time.Second,
	}

	err := cmd.Run(context.Background())
	if err != nil {
		fmt.Println("Run err:", err)

		return
	}

	err = cmd.Signal(os.Interrupt)
	if err != nil {
		fmt.Println("Signal err:", err)

		return
	}

	exec, err := cmd.Wait()
	fmt.Println("Wait err:", err)
	fmt.Println("Exit code:", exec.ExitCode)
	fmt.Println("Stdout:")
	fmt.Println(exec.Stdout)
	fmt.Println("Stderr:")
	fmt.Println(exec.Stderr)
	fmt.Println("Signal:", exec.Signal)

	// Output:
	// Wait err: command execution signaled
	// Exit code: -1
	// Stdout:
	//
	// Stderr:
	//
	// Signal: interrupt
}

func ExampleCommand_WithPTY() {
	cmd := &com.Command{
		Binary: "bash",
		Args: []string{
			"-c",
			"--",
			"[ -t 1 ] || { echo not a pty; exit 41; }; printf onstdout; >&2 printf onstderr;",
		},
		Timeout: 1 * time.Second,
	}

	// The PTY can be set to any of stdin, stdout, stderr
	// Note that PTY are supported only on Linux, Darwin and FreeBSD
	cmd.WithPTY(false, true, false)

	err := cmd.Run(context.Background())
	if err != nil {
		fmt.Println("Run err:", err)

		return
	}

	exec, err := cmd.Wait()
	if err != nil {
		fmt.Println("Wait err:", err)

		return
	}

	fmt.Println("Exit code:", exec.ExitCode)
	fmt.Println("Stdout:")
	fmt.Println(exec.Stdout)
	fmt.Println("Stderr:")
	fmt.Println(exec.Stderr)

	// Output:
	// Exit code: 0
	// Stdout:
	// onstdout
	// Stderr:
	// onstderr
}

func ExampleCommand_Feed() {
	cmd := &com.Command{
		Binary: "bash",
		Args: []string{
			"-c", "--",
			"read line1; read line2; printf 'from stdin%s%s%s' \"$line1\" \"$line2\";",
		},
	}

	// Use WithFeeder if you do want to perform additional tasks before feeding to stdin
	cmd.WithFeeder(func() io.Reader {
		time.Sleep(100 * time.Millisecond)

		return strings.NewReader("hello world\n")
	})

	// Or use the simpler Feed if you just want to pass along content to stdin
	// Note that successive calls to WithFeeder / Feed will be written to stdin in order.
	cmd.Feed(strings.NewReader("hello again\n"))

	err := cmd.Run(context.Background())
	if err != nil {
		fmt.Println("Run err:", err)

		return
	}

	exec, err := cmd.Wait()
	if err != nil {
		fmt.Println("Wait err:", err)

		return
	}

	fmt.Println("Exit code:", exec.ExitCode)
	fmt.Println("Stdout:")
	fmt.Println(exec.Stdout)
	fmt.Println("Stderr:")
	fmt.Println(exec.Stderr)

	// Output:
	// Exit code: 0
	// Stdout:
	// from stdinhello worldhello again
	// Stderr:
	//
}

func ExampleErrTimeout() {
	cmd := &com.Command{
		Binary:  "sleep",
		Args:    []string{"3600"},
		Timeout: time.Second,
	}

	err := cmd.Run(context.Background())
	if err != nil {
		fmt.Println("Run err:", err)

		return
	}

	exec, err := cmd.Wait()
	fmt.Println("Wait err:", err)
	fmt.Println("Exit code:", exec.ExitCode)
	fmt.Println("Stdout:")
	fmt.Println(exec.Stdout)
	fmt.Println("Stderr:")
	fmt.Println(exec.Stderr)

	// Output:
	// Wait err: command timed out
	// Exit code: -1
	// Stdout:
	//
	// Stderr:
	//
}

func ExampleErrFailedStarting() {
	cmd := &com.Command{
		Binary: "non-existent",
	}

	err := cmd.Run(context.Background())

	fmt.Println("Run err:")
	fmt.Println(err)

	// Output:
	// Run err:
	// command failed starting
	// exec: "non-existent": executable file not found in $PATH
}

func ExampleErrExecutionFailed() {
	cmd := &com.Command{
		Binary: "bash",
		Args:   []string{"-c", "--", "does-not-exist"},
	}

	err := cmd.Run(context.Background())
	if err != nil {
		fmt.Println("Run err:", err)

		return
	}

	exec, err := cmd.Wait()
	fmt.Println("Wait err:", err)
	fmt.Println("Exit code:", exec.ExitCode)

	// Output:
	// Wait err: command returned a non-zero exit code
	// Exit code: 127
}
