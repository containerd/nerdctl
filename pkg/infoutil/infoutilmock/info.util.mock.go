//go:build windows

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

package infoutilmock

import (
	"reflect"

	"go.uber.org/mock/gomock"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// MockWindowsInfoUtil is a mock of windowsInfoUtil interface
type MockWindowsInfoUtil struct {
	ctrl     *gomock.Controller
	recorder *MockWindowsInfoUtilMockRecorder
}

// MockWindowsInfoUtilMockRecorder is the mock recorder for MockWindowsInfoUtil
type MockWindowsInfoUtilMockRecorder struct {
	mock *MockWindowsInfoUtil
}

// NewMockWindowsInfoUtil creates a new mock instance
func NewMockWindowsInfoUtil(ctrl *gomock.Controller) *MockWindowsInfoUtil {
	mock := &MockWindowsInfoUtil{ctrl: ctrl}
	mock.recorder = &MockWindowsInfoUtilMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use
func (m *MockWindowsInfoUtil) EXPECT() *MockWindowsInfoUtilMockRecorder {
	return m.recorder
}

// Create mocks the RtlGetVersion method of windowsInfoUtil
func (m *MockWindowsInfoUtil) RtlGetVersion() *windows.OsVersionInfoEx {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RtlGetVersion")
	ret0, _ := ret[0].(*windows.OsVersionInfoEx)
	return ret0
}

// Expected call of RtlGetVersion
func (m *MockWindowsInfoUtilMockRecorder) RtlGetVersion() *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(
		m.mock,
		"RtlGetVersion",
		reflect.TypeOf((*MockWindowsInfoUtil)(nil).RtlGetVersion),
	)
}

// Create mocks the GetRegistryStringValue method of windowsInfoUtil
func (m *MockWindowsInfoUtil) GetRegistryStringValue(key registry.Key, path string, name string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRegistryStringValue", key, path, name)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Expected call of GetRegistryStringValue
func (m *MockWindowsInfoUtilMockRecorder) GetRegistryStringValue(key any, path any, name any) *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(
		m.mock,
		"GetRegistryStringValue",
		reflect.TypeOf((*MockWindowsInfoUtil)(nil).GetRegistryStringValue),
		key, path, name,
	)
}

// Create mocks the GetRegistryIntValue method of windowsInfoUtil
func (m *MockWindowsInfoUtil) GetRegistryIntValue(key registry.Key, path string, name string) (int, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetRegistryIntValue", key, path, name)
	ret0, _ := ret[0].(int)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Expected call of GetRegistryIntValue
func (m *MockWindowsInfoUtilMockRecorder) GetRegistryIntValue(key any, path any, name any) *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(
		m.mock,
		"GetRegistryIntValue",
		reflect.TypeOf((*MockWindowsInfoUtil)(nil).GetRegistryIntValue),
		key, path, name,
	)
}
