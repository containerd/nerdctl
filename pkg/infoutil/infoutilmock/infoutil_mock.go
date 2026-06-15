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

// Package infoutilmock provides a hand-rolled fake for the windowsInfoUtil
// interface used in tests. It replaces the previous gomock-generated mock so
// that nerdctl does not depend on go.uber.org/mock.
package infoutilmock

import (
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// FakeWindowsInfoUtil is a configurable test double for the windowsInfoUtil
// interface. Tests assign the function fields to control behavior per call.
type FakeWindowsInfoUtil struct {
	RtlGetVersionFunc          func() *windows.OsVersionInfoEx
	GetRegistryStringValueFunc func(key registry.Key, path string, name string) (string, error)
	GetRegistryIntValueFunc    func(key registry.Key, path string, name string) (int, error)
}

// NewFakeWindowsInfoUtil returns an empty fake. Callers populate the function
// fields they need for a given test.
func NewFakeWindowsInfoUtil() *FakeWindowsInfoUtil {
	return &FakeWindowsInfoUtil{}
}

// RtlGetVersion calls the configured function, or returns nil if unset.
func (f *FakeWindowsInfoUtil) RtlGetVersion() *windows.OsVersionInfoEx {
	if f.RtlGetVersionFunc == nil {
		return nil
	}
	return f.RtlGetVersionFunc()
}

// GetRegistryStringValue calls the configured function, or returns ("", nil) if unset.
func (f *FakeWindowsInfoUtil) GetRegistryStringValue(key registry.Key, path string, name string) (string, error) {
	if f.GetRegistryStringValueFunc == nil {
		return "", nil
	}
	return f.GetRegistryStringValueFunc(key, path, name)
}

// GetRegistryIntValue calls the configured function, or returns (0, nil) if unset.
func (f *FakeWindowsInfoUtil) GetRegistryIntValue(key registry.Key, path string, name string) (int, error) {
	if f.GetRegistryIntValueFunc == nil {
		return 0, nil
	}
	return f.GetRegistryIntValueFunc(key, path, name)
}
