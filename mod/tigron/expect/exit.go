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

package expect

const (
	// ExitCodeSuccess will ensure that the command effectively ran returned with exit code zero.
	ExitCodeSuccess = 0
	// ExitCodeGenericFail will verify that the command ran and exited with a non-zero error code.
	// This does NOT include timeouts, cancellation, or signals.
	ExitCodeGenericFail = -10
	// ExitCodeNoCheck does not enforce any check at all on the function.
	ExitCodeNoCheck = -11
	// ExitCodeTimeout verifies that the command was cancelled on timeout.
	ExitCodeTimeout = -12
	// ExitCodeSignaled verifies that the command has been terminated by a signal.
	ExitCodeSignaled = -13
	// ExitCodeCancelled = -14.
)
