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

package test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/containerd/nerdctl/mod/tigron/internal/assertive"
	"github.com/containerd/nerdctl/mod/tigron/tig"
)

const (
	identifierMaxLength       = 76
	identifierSeparator       = "-"
	identifierSignatureLength = 8
)

// WithData returns a data object with a certain key value set.
func WithData(key, value string) Data {
	dat := &data{}
	dat.Set(key, value)

	return dat
}

// Contains the implementation of the Data interface
//
//nolint:varnamelen
func configureData(t tig.T, seedData, parent Data) Data {
	t.Helper()

	silentT := assertive.WithSilentSuccess(t)

	if seedData == nil {
		seedData = &data{}
	}

	var labels map[string]string
	if castData, ok := seedData.(*data); ok {
		labels = castData.labels
	}

	dat := &data{
		// Note: implementation dependent
		labels: labels,
		testID: func(suffix ...string) string {
			suffix = append([]string{t.Name()}, suffix...)

			return defaultIdentifierHashing(suffix...)
		},
		t: silentT,
	}

	// NOTE: certain systems will use the path dirname to decide how they name resources.
	// t.TempDir() will always return /tmp/TestTempDir2153252249/001, meaning these systems will all
	// use the identical 001 part. This is true for compose specifically.
	// Appending the base test identifier here would guarantee better unicity.
	// Note though that Windows will barf if >256 characters, so, hashing...
	// Small caveat: identically named tests in different modules WILL still end-up with the same last segment.
	tempDir := filepath.Join(
		t.TempDir(),
		fmt.Sprintf("%x", sha256.Sum256([]byte(t.Name())))[0:identifierSignatureLength],
	)

	assertive.ErrorIsNil(silentT, os.MkdirAll(tempDir, DirPermissionsDefault))

	dat.tempDir = tempDir

	if parent != nil {
		dat.adopt(parent)
	}

	return dat
}

type data struct {
	labels  map[string]string
	testID  func(suffix ...string) string
	tempDir string
	t       tig.T
}

func (dt *data) Get(key string) string {
	return dt.labels[key]
}

func (dt *data) Set(key, value string) Data {
	if dt.labels == nil {
		dt.labels = map[string]string{}
	}

	dt.labels[key] = value

	return dt
}

func (dt *data) AssetLoad(key string) string {
	//nolint:gosec
	content, err := os.ReadFile(filepath.Join(dt.tempDir, key))

	assertive.ErrorIsNil(dt.t, err)

	return string(content)
}

func (dt *data) AssetSave(key, value string) Data {
	err := os.WriteFile(
		filepath.Join(dt.tempDir, key),
		[]byte(value),
		FilePermissionsDefault,
	)

	assertive.ErrorIsNil(dt.t, err)

	return dt
}

func (dt *data) AssetPath(key string) string {
	return filepath.Join(dt.tempDir, key)
}

func (dt *data) Identifier(suffix ...string) string {
	return dt.testID(suffix...)
}

func (dt *data) TempDir() string {
	return dt.tempDir
}

func (dt *data) adopt(parent Data) {
	// Note: implementation dependent
	if castData, ok := parent.(*data); ok {
		for k, v := range castData.labels {
			// Only copy keys that are not set already
			if _, ok := dt.labels[k]; !ok {
				dt.Set(k, v)
			}
		}
	}
}

func defaultIdentifierHashing(names ...string) string {
	// Notes: identifier MAY be used for namespaces, image names, etc.
	// So, the rules are stringent on what it can contain.
	replaceWith := []byte(identifierSeparator)
	name := strings.ToLower(strings.Join(names, string(replaceWith)))
	// Ensure we have a unique identifier despite characters replacements
	// (well, as unique as the names collection being passed)
	signature := fmt.Sprintf("%x", sha256.Sum256([]byte(name)))[0:identifierSignatureLength]
	// Make sure we do not use any unsafe characters
	safeName := regexp.MustCompile(`[^a-z0-9-]+`)
	// And we avoid repeats of the separator
	noRepeat := regexp.MustCompile(fmt.Sprintf(`[%s]{2,}`, replaceWith))
	escapedName := safeName.ReplaceAll([]byte(name), replaceWith)
	escapedName = noRepeat.ReplaceAll(escapedName, replaceWith)
	// Do not allow trailing or leading dash (as that may stutter)
	name = strings.Trim(string(escapedName), string(replaceWith))

	// Ensure we will never go above 76 characters in length (with signature)
	if len(name) > (identifierMaxLength - len(signature)) {
		name = name[0 : identifierMaxLength-identifierSignatureLength-len(identifierSeparator)]
	}

	if name[len(name)-1:] != identifierSeparator {
		signature = identifierSeparator + signature
	}

	return name + signature
}
