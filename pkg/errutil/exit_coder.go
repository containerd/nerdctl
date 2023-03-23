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

package errutil

import "os"

type ExitCoder interface {
	error
	ExitCode() int
}

// ExitCodeError is to allow the program to exit with status code without outputting an error message.
type ExitCodeError struct {
	exitCode int
}

func NewExitCoderErr(exitCode int) ExitCodeError {
	return ExitCodeError{
		exitCode: exitCode,
	}
}

func (e ExitCodeError) ExitCode() int {
	return e.exitCode
}

func (e ExitCodeError) Error() string {
	return ""
}

func HandleExitCoder(err error) {
	if err == nil {
		return
	}

	if exitErr, ok := err.(ExitCoder); ok {
		os.Exit(exitErr.ExitCode())
	}
}
