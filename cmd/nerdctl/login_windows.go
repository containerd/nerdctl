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

package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

func readPassword() (string, error) {
	var fd int
	if term.IsTerminal(int(syscall.Stdin)) {
		fd = int(syscall.Stdin)
	} else {
		return "", fmt.Errorf("error allocating terminal")
	}
	bytePassword, err := term.ReadPassword(fd)
	if err != nil {
		return "", fmt.Errorf("error reading password: %w", err)
	}

	return string(bytePassword), nil
}

func readUsername() (string, error) {
	var fd *os.File
	if term.IsTerminal(int(syscall.Stdin)) {
		fd = os.Stdin
	} else {
		return "", fmt.Errorf("error allocating terminal")
	}

	reader := bufio.NewReader(fd)
	username, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("error reading username: %w", err)
	}
	username = strings.TrimSpace(username)

	return username, nil
}
