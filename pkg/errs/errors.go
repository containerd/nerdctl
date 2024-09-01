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

package errs

import (
	"errors"

	"github.com/containerd/errdefs"
)

var (
	// ErrSystemIsBroken should wrap all system-level errors (filesystem unexpected conditions, hosed files, misbehaving subsystems)
	ErrSystemIsBroken = errors.New("system error")

	// ErrInvalidArgument should wrap all cases where an argument does not match expected syntax,
	// or prevents an operation from succeeding because of its value
	ErrInvalidArgument = errdefs.ErrInvalidArgument

	// ErrNotFound is the generic error describing that a requested resource could not be found
	ErrNotFound = errdefs.ErrNotFound

	// ErrNetworkCondition is meant to wrap network level errors - DNS, TCP, TLS errors
	// but NOT http server level errors
	ErrNetworkCondition = errors.New("network communication failed")

	// ErrServerIsMisbehaving should wrap all server errors (eg: status code 50x)
	// but NOT dns, tcp, or tls errors
	ErrServerIsMisbehaving = errors.New("server error")

	// ErrFailedPrecondition should wrap errors encountered when a certain operation cannot be performed because
	// a precondition prevents it from completing. For example, removing a volume that is in use.
	ErrFailedPrecondition = errors.New("unable to perform the requested operation")
)
