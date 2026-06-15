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

package login

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/containerd/nerdctl/v2/pkg/imgutil/dockerconfigresolver"
)

var (
	// User did not provide non-empty credentials when prompted for it
	ErrUsernameIsRequired = errors.New("username is required")
	ErrPasswordIsRequired = errors.New("password is required")

	// System errors - not a terminal, failure to read, etc
	ErrReadingUsername        = errors.New("unable to read username")
	ErrReadingPassword        = errors.New("unable to read password")
	ErrNotATerminal           = errors.New("stdin is not a terminal (Hint: use `nerdctl login --username=USERNAME --password-stdin`)")
	ErrCannotAllocateTerminal = errors.New("error allocating terminal")
)

// promptUserForAuthentication will prompt the user for credentials if needed
// It might error with any of the errors defined above.
func promptUserForAuthentication(credentials *dockerconfigresolver.Credentials, username, password string, stdout io.Writer) error {
	var err error

	// If the provided username is empty...
	if username = strings.TrimSpace(username); username == "" {
		// Use the one we know of (from the store)
		username = credentials.Username
		// If the one from the store was empty as well, prompt and read the username
		if username == "" {
			_, _ = fmt.Fprint(stdout, "Enter Username: ")
			username, err = readUsername()
			if err != nil {
				return err
			}

			username = strings.TrimSpace(username)
			// If it still is empty, that is an error
			if username == "" {
				return ErrUsernameIsRequired
			}
		}
	}

	// If password was NOT passed along, ask for it
	if password == "" {
		_, _ = fmt.Fprint(stdout, "Enter Password: ")
		password, err = readPassword()
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintln(stdout)
		password = strings.TrimSpace(password)

		// If nothing was provided, error out
		if password == "" {
			return ErrPasswordIsRequired
		}
	}

	// Attach non-empty credentials to the auth object and return
	credentials.Username = username
	credentials.Password = password

	return nil
}

// readUsername will try to read from user input
// It might error with:
// - ErrNotATerminal
// - ErrReadingUsername
func readUsername() (string, error) {
	fd := os.Stdin
	if !term.IsTerminal(int(fd.Fd())) {
		return "", ErrNotATerminal
	}

	username, err := bufio.NewReader(fd).ReadString('\n')
	if err != nil {
		return "", errors.Join(ErrReadingUsername, err)
	}

	return strings.TrimSpace(username), nil
}
