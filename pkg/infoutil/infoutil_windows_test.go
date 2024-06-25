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

package infoutil

import (
	"testing"

	"go.uber.org/mock/gomock"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"gotest.tools/v3/assert"

	mocks "github.com/containerd/nerdctl/v2/pkg/infoutil/infoutilmock"
)

func setUpMocks(t *testing.T) *mocks.MockWindowsInfoUtil {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInfoUtil := mocks.NewMockWindowsInfoUtil(ctrl)

	// Mock registry value: CurrentBuildNumber
	mockInfoUtil.
		EXPECT().
		GetRegistryStringValue(gomock.Any(), gomock.Any(), "CurrentBuildNumber").
		Return("19041", nil).
		AnyTimes()

	// Mock registry value: DisplayVersion
	mockInfoUtil.
		EXPECT().
		GetRegistryStringValue(gomock.Any(), gomock.Any(), "DisplayVersion").
		Return("22H4", nil).
		AnyTimes()

	// Mock registry value: UBR
	mockInfoUtil.
		EXPECT().
		GetRegistryIntValue(gomock.Any(), gomock.Any(), "UBR").
		Return(558, nil).
		AnyTimes()

	return mockInfoUtil
}

const (
	verNTWorkStation      = 0x0000001
	verNTDomainController = 0x0000002
)

func TestDistroName(t *testing.T) {
	mockInfoUtil := setUpMocks(t)

	baseVersion := windows.OsVersionInfoEx{
		MajorVersion: 10,
		MinorVersion: 0,
		BuildNumber:  19041,
	}

	tests := []struct {
		productType byte
		expected    string
	}{
		{
			productType: verNTWorkStation,
			expected:    "Microsoft Windows Version 22H4 (OS Build 19041.558)",
		},
		{
			productType: verNTServer,
			expected:    "Microsoft Windows Server Version 22H4 (OS Build 19041.558)",
		},
	}

	for _, tt := range tests {
		// Mock sys/windows RtlGetVersion
		osvi := baseVersion
		osvi.ProductType = tt.productType
		mockInfoUtil.EXPECT().RtlGetVersion().Return(&osvi).Times(1)

		t.Run(tt.expected, func(t *testing.T) {
			actual, err := distroName(mockInfoUtil)
			assert.Equal(t, tt.expected, actual, "distroName should return the name of the operating system")
			assert.NilError(t, err)
		})
	}
}

func TestDistroNameError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInfoUtil := mocks.NewMockWindowsInfoUtil(ctrl)

	mockInfoUtil.EXPECT().RtlGetVersion().Return(nil).Times(0)
	mockInfoUtil.
		EXPECT().
		GetRegistryStringValue(gomock.Any(), gomock.Any(), gomock.Any()).
		Return("19041", registry.ErrNotExist).AnyTimes()

	actual, err := distroName(mockInfoUtil)
	assert.ErrorContains(t, err, registry.ErrNotExist.Error(), "distroName should return an error on error")
	assert.Equal(t, "", actual, "distroname should return an empty string on error")
}

func TestGetKernelVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInfoUtil := mocks.NewMockWindowsInfoUtil(ctrl)

	// Mock registry value: BuildLabEx
	mockInfoUtil.
		EXPECT().
		GetRegistryStringValue(gomock.Any(), gomock.Any(), "BuildLabEx").
		Return("10240.16412.amd64fre.th1.150729-1800", nil).
		Times(1)

	baseVersion := windows.OsVersionInfoEx{
		MajorVersion: 10,
		MinorVersion: 0,
		BuildNumber:  19041,
	}

	expected := "10.0 19041 (10240.16412.amd64fre.th1.150729-1800)"

	// Mock sys/windows RtlGetVersion
	osvi := baseVersion
	mockInfoUtil.EXPECT().RtlGetVersion().Return(&osvi).Times(1)

	actual, err := getKernelVersion(mockInfoUtil)
	assert.NilError(t, err)
	assert.Equal(t, expected, actual, "getKernelVersion should return the kernel version")
}

func TestGetKernelVersionError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockInfoUtil := mocks.NewMockWindowsInfoUtil(ctrl)

	mockInfoUtil.EXPECT().RtlGetVersion().Return(nil).Times(0)
	mockInfoUtil.
		EXPECT().
		GetRegistryStringValue(gomock.Any(), gomock.Any(), gomock.Any()).
		Return("", registry.ErrNotExist).Times(1)

	actual, err := getKernelVersion(mockInfoUtil)
	assert.ErrorContains(t, err, registry.ErrNotExist.Error(), "getKernelVersion should return an error on error")
	assert.Equal(t, "", actual, "getKernelVersion should return an empty string on error")
}

func TestIsWindowsServer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	tests := []struct {
		productType string
		osvi        windows.OsVersionInfoEx
		expected    bool
	}{
		{
			productType: "VER_NT_WORKSTATION",
			osvi:        windows.OsVersionInfoEx{ProductType: verNTWorkStation},
			expected:    false,
		},
		{
			productType: "VER_NT_DOMAIN_CONTROLLER",
			osvi:        windows.OsVersionInfoEx{ProductType: verNTDomainController},
			expected:    false,
		},
		{
			productType: "VER_NT_SERVER",
			osvi:        windows.OsVersionInfoEx{ProductType: verNTServer},
			expected:    true,
		},
	}

	mockSysCall := mocks.NewMockWindowsInfoUtil(ctrl)
	for _, tt := range tests {
		mockSysCall.EXPECT().RtlGetVersion().Return(&tt.osvi)

		t.Run(tt.productType, func(t *testing.T) {
			actual := isWindowsServer(mockSysCall)
			assert.Equal(t, tt.expected, actual, "isWindowsServer should return true on Windows Server")
		})
	}
}
