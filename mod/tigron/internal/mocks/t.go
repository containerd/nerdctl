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

//nolint:forcetypeassert
//revive:disable:exported,max-public-structs,package-comments
package mocks

// FIXME: type asserts...

import (
	"github.com/containerd/nerdctl/mod/tigron/internal/mimicry"
	"github.com/containerd/nerdctl/mod/tigron/tig"
)

type T interface {
	tig.T
	mimicry.Consumer
}

type (
	THelperIn  struct{}
	THelperOut struct{}

	TFailIn  struct{}
	TFailOut struct{}

	TFailNowIn  struct{}
	TFailNowOut struct{}

	TLogIn  []any
	TLogOut struct{}

	TNameIn  struct{}
	TNameOut = string

	TTempDirIn  struct{}
	TTempDirOut = string
)

type MockT struct {
	mimicry.Core
}

func (m *MockT) Helper() {
	if handler := m.Retrieve(); handler != nil {
		handler.(mimicry.Function[THelperIn, THelperOut])(THelperIn{})
	}
}

func (m *MockT) FailNow() {
	if handler := m.Retrieve(); handler != nil {
		handler.(mimicry.Function[TFailNowIn, TFailNowOut])(TFailNowIn{})
	}
}

func (m *MockT) Fail() {
	if handler := m.Retrieve(); handler != nil {
		handler.(mimicry.Function[TFailIn, TFailOut])(TFailIn{})
	}
}

func (m *MockT) Log(args ...any) {
	if handler := m.Retrieve(args...); handler != nil {
		handler.(mimicry.Function[TLogIn, TLogOut])(args)
	}
}

func (m *MockT) Name() string {
	if handler := m.Retrieve(); handler != nil {
		return handler.(mimicry.Function[TNameIn, TNameOut])(TNameIn{})
	}

	return ""
}

func (m *MockT) TempDir() string {
	if handler := m.Retrieve(); handler != nil {
		return handler.(mimicry.Function[TTempDirIn, TTempDirOut])(TTempDirIn{})
	}

	return ""
}
