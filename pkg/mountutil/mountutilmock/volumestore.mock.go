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

package mountutilmock

import (
	"reflect"

	"github.com/containerd/nerdctl/v2/pkg/inspecttypes/native"
	"go.uber.org/mock/gomock"
)

// MockVolumeStore is a mock of VolumeStore interface
type MockVolumeStore struct {
	ctrl     *gomock.Controller
	recorder *MockVolumeStoreMockRecorder
}

// MockVolumeStoreMockRecorder is the mock recorder for MockVolumeStore
type MockVolumeStoreMockRecorder struct {
	mock *MockVolumeStore
}

// NewMockVolumeStore creates a new mock instance
func NewMockVolumeStore(ctrl *gomock.Controller) *MockVolumeStore {
	mock := &MockVolumeStore{ctrl: ctrl}
	mock.recorder = &MockVolumeStoreMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockVolumeStore) EXPECT() *MockVolumeStoreMockRecorder {
	return m.recorder
}

// Create mocks the Create method of VolumeStore
func (m *MockVolumeStore) Create(name string, labels []string) (*native.Volume, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Create", name, labels)
	ret0, _ := ret[0].(*native.Volume)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Create indicates an expected call of Create
func (m *MockVolumeStoreMockRecorder) Create(name any, labels any) *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(m.mock, "Create", reflect.TypeOf((*MockVolumeStore)(nil).Create), name, labels)
}

// Get mocks the Get method of VolumeStore
func (m *MockVolumeStore) Get(name string, size bool) (*native.Volume, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", name, size)
	ret0, _ := ret[0].(*native.Volume)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get
func (m *MockVolumeStoreMockRecorder) Get(name any, size any) *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(m.mock, "Get", reflect.TypeOf((*MockVolumeStore)(nil).Get), name, size)
}

// List mocks the List method of VolumeStore
func (m *MockVolumeStore) List(size bool) (map[string]native.Volume, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "List", size)
	ret0, _ := ret[0].(map[string]native.Volume)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// List indicates an expected call of List
func (m *MockVolumeStoreMockRecorder) List(size any) *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(m.mock, "List", reflect.TypeOf((*MockVolumeStore)(nil).List), size)
}

// Remove mocks the Remove method of VolumeStore
func (m *MockVolumeStore) Remove(names []string) ([]string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Remove", names)
	ret0, _ := ret[0].([]string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Remove indicates an expected call of Remove
func (m *MockVolumeStoreMockRecorder) Remove(names any) *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(m.mock, "Remove", reflect.TypeOf((*MockVolumeStore)(nil).Remove), names)
}

// Dir mocks the Dir method of VolumeStore
func (m *MockVolumeStore) Dir() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Dir")
	ret0, _ := ret[0].(string)
	return ret0
}

// Dir indicates an expected call of Dir
func (m *MockVolumeStoreMockRecorder) Dir() *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(m.mock, "Dir", reflect.TypeOf((*MockVolumeStore)(nil).Dir))
}
