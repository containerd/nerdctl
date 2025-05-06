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
	"io"
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

// WithLabels returns a Data object with specific key value labels set.
func WithLabels(in map[string]string) Data {
	dat := &data{
		labels: &labels{
			inMap: in,
		},
		temp: &temp{},
	}

	return dat
}

type labels struct {
	inMap map[string]string
}

func (lb *labels) Get(key string) string {
	return lb.inMap[key]
}

func (lb *labels) Set(key, value string) {
	lb.inMap[key] = value
}

type temp struct {
	tempDir string
	t       tig.T
}

func (tp *temp) Load(key ...string) string {
	tp.t.Helper()

	pth := filepath.Join(append([]string{tp.tempDir}, key...)...)

	//nolint:gosec // Fine in the context of testing
	content, err := os.ReadFile(pth)

	assertive.ErrorIsNil(
		assertive.WithSilentSuccess(tp.t),
		err,
		fmt.Sprintf("Loading file %q must succeed", pth),
	)

	return string(content)
}

func (tp *temp) Exists(key ...string) {
	tp.t.Helper()

	pth := filepath.Join(append([]string{tp.tempDir}, key...)...)

	_, err := os.Stat(pth)

	assertive.ErrorIsNil(
		assertive.WithSilentSuccess(tp.t),
		err,
		fmt.Sprintf("File %q must exist", pth),
	)
}

func (tp *temp) Save(value string, key ...string) string {
	tp.t.Helper()

	tp.Dir(key[:len(key)-1]...)

	pth := filepath.Join(append([]string{tp.tempDir}, key...)...)

	err := os.WriteFile(
		pth,
		[]byte(value),
		FilePermissionsDefault,
	)

	assertive.ErrorIsNil(
		assertive.WithSilentSuccess(tp.t),
		err,
		fmt.Sprintf("Saving file %q must succeed", pth),
	)

	return pth
}

func (tp *temp) SaveToWriter(writer func(file io.Writer) error, key ...string) string {
	tp.t.Helper()

	tp.Dir(key[:len(key)-1]...)

	pth := filepath.Join(append([]string{tp.tempDir}, key...)...)
	silentT := assertive.WithSilentSuccess(tp.t)

	//nolint:gosec // it is fine
	file, err := os.OpenFile(pth, os.O_CREATE, FilePermissionsDefault)
	assertive.ErrorIsNil(
		silentT,
		err,
		fmt.Sprintf("Opening file %q must succeed", pth),
	)

	defer func() {
		err = file.Close()
		assertive.ErrorIsNil(
			silentT,
			err,
			fmt.Sprintf("Closing file %q must succeed", pth),
		)
	}()

	err = writer(file)
	assertive.ErrorIsNil(
		silentT,
		err,
		fmt.Sprintf("Filewriter failed while attempting to write to %q", pth),
	)

	return pth
}

func (tp *temp) Dir(key ...string) string {
	tp.t.Helper()

	pth := filepath.Join(append([]string{tp.tempDir}, key...)...)
	err := os.MkdirAll(pth, DirPermissionsDefault)

	assertive.ErrorIsNil(
		assertive.WithSilentSuccess(tp.t),
		err,
		fmt.Sprintf("Creating directory %q must succeed", pth),
	)

	return pth
}

func (tp *temp) Path(key ...string) string {
	tp.t.Helper()

	return filepath.Join(append([]string{tp.tempDir}, key...)...)
}

type data struct {
	temp   DataTemp
	labels DataLabels
	testID func(suffix ...string) string
}

func (dt *data) Identifier(suffix ...string) string {
	return dt.testID(suffix...)
}

func (dt *data) Labels() DataLabels {
	return dt.labels
}

func (dt *data) Temp() DataTemp {
	return dt.temp
}

// Contains the implementation of the Data interface
//
//nolint:varnamelen
func newData(t tig.T, seed, parent Data) Data {
	t.Helper()

	t = assertive.WithSilentSuccess(t)

	seedMap := map[string]string{}

	if seed != nil {
		if inLab, ok := seed.Labels().(*labels); ok {
			seedMap = inLab.inMap
		}
	}

	if parent != nil {
		for k, v := range parent.Labels().(*labels).inMap {
			// Only copy keys that are not set already
			if _, ok := seedMap[k]; !ok {
				seedMap[k] = v
			}
		}
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

	assertive.ErrorIsNil(t, os.MkdirAll(tempDir, DirPermissionsDefault))

	dat := &data{
		labels: &labels{
			inMap: seedMap,
		},
		temp: &temp{
			tempDir: tempDir,
			t:       t,
		},
		testID: func(suffix ...string) string {
			suffix = append([]string{t.Name()}, suffix...)

			return defaultIdentifierHashing(suffix...)
		},
	}

	return dat
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
