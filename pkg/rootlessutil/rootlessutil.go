/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

package rootlessutil

import (
	"os"
	"strconv"

	"github.com/pkg/errors"
)

func IsRootless() bool {
	return IsRootlessParent() || IsRootlessChild()
}

func ParentEUID() int {
	if !IsRootlessChild() {
		return os.Geteuid()
	}
	env := os.Getenv("ROOTLESSKIT_PARENT_EUID")
	if env == "" {
		panic("environment variable ROOTLESSKIT_PARENT_EUID is not set")
	}
	i, err := strconv.Atoi(env)
	if err != nil {
		panic(errors.Wrapf(err, "failed to parse ROOTLESSKIT_PARENT_EUID=%q", env))
	}
	return i
}

func ParentEGID() int {
	if !IsRootlessChild() {
		return os.Getegid()
	}
	env := os.Getenv("ROOTLESSKIT_PARENT_EGID")
	if env == "" {
		panic("environment variable ROOTLESSKIT_PARENT_EGID is not set")
	}
	i, err := strconv.Atoi(env)
	if err != nil {
		panic(errors.Wrapf(err, "failed to parse ROOTLESSKIT_PARENT_EGID=%q", env))
	}
	return i
}
