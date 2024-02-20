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

package builder

import (
	"reflect"
	"testing"

	"github.com/containerd/containerd/platforms"
	"go.uber.org/mock/gomock"
	"gotest.tools/v3/assert"
)

type MockParse struct {
	ctrl     *gomock.Controller
	recorder *MockParseRecorder
}

type MockParseRecorder struct {
	mock *MockParse
}

func newMockParser(ctrl *gomock.Controller) *MockParse {
	mock := &MockParse{ctrl: ctrl}
	mock.recorder = &MockParseRecorder{mock}
	return mock
}

func (m *MockParse) EXPECT() *MockParseRecorder {
	return m.recorder
}

func (m *MockParse) Parse(platform string) (platforms.Platform, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Parse")
	ret0, _ := ret[0].(platforms.Platform)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

func (m *MockParseRecorder) Parse(platform string) *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(m.mock, "Parse", reflect.TypeOf((*MockParse)(nil).Parse))
}

func (m *MockParse) DefaultSpec() platforms.Platform {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DefaultSpec")
	ret0, _ := ret[0].(platforms.Platform)
	return ret0
}

func (m *MockParseRecorder) DefaultSpec() *gomock.Call {
	m.mock.ctrl.T.Helper()
	return m.mock.ctrl.RecordCallWithMethodType(m.mock, "DefaultSpec", reflect.TypeOf((*MockParse)(nil).DefaultSpec))
}

func TestIsMatchingRuntimePlatform(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		mock func(*MockParse)
		want bool
	}{
		{
			name: "Image is shareable when Runtime and build platform match for os, arch and variant",
			mock: func(mockParser *MockParse) {
				mockParser.EXPECT().Parse("test").Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}, nil)
				mockParser.EXPECT().DefaultSpec().Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"})
			},
			want: true,
		},
		{
			name: "Image is shareable when Runtime and build platform match for os, arch. Variant is not defined",
			mock: func(mockParser *MockParse) {
				mockParser.EXPECT().Parse("test").Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: ""}, nil)
				mockParser.EXPECT().DefaultSpec().Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"})
			},
			want: true,
		},
		{
			name: "Image is not shareable when Runtime and build platform donot math OS",
			mock: func(mockParser *MockParse) {
				mockParser.EXPECT().Parse("test").Return(platforms.Platform{OS: "OS", Architecture: "mockArch", Variant: ""}, nil)
				mockParser.EXPECT().DefaultSpec().Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"})
			},
			want: false,
		},
		{
			name: "Image is not shareable when Runtime and build platform donot math Arch",
			mock: func(mockParser *MockParse) {
				mockParser.EXPECT().Parse("test").Return(platforms.Platform{OS: "mockOS", Architecture: "Arch", Variant: ""}, nil)
				mockParser.EXPECT().DefaultSpec().Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"})
			},
			want: false,
		},
		{
			name: "Image is not shareable when Runtime and build platform donot math Variant",
			mock: func(mockParser *MockParse) {
				mockParser.EXPECT().Parse("test").Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "Variant"}, nil)
				mockParser.EXPECT().DefaultSpec().Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"})
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockParser := newMockParser(ctrl)
			tc.mock(mockParser)
			r := isMatchingRuntimePlatform("test", mockParser)
			assert.Equal(t, r, tc.want, tc.name)
		})
	}
}

func TestIsBuildPlatformDefault(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		mock     func(*MockParse)
		platform []string
		want     bool
	}{
		{
			name:     "Image is shreable when len of platform is 0",
			platform: make([]string, 0),
			want:     true,
		},
		{
			name:     "Image is shareable when Runtime and build platform match for os, arch and variant",
			platform: []string{"test"},
			mock: func(mockParser *MockParse) {
				mockParser.EXPECT().Parse("test").Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}, nil)
				mockParser.EXPECT().DefaultSpec().Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"})
			},
			want: true,
		},
		{
			name:     "Image is not shareable when Runtime build platform dont match",
			platform: []string{"test"},
			mock: func(mockParser *MockParse) {
				mockParser.EXPECT().Parse("test").Return(platforms.Platform{OS: "OS", Architecture: "mockArch", Variant: "mockVariant"}, nil)
				mockParser.EXPECT().DefaultSpec().Return(platforms.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"})
			},
			want: false,
		},
		{
			name:     "Image is not shareable when more than 2 platforms are passed",
			platform: []string{"test1", "test2"},
			want:     false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockParser := newMockParser(ctrl)
			if len(tc.platform) == 1 {
				tc.mock(mockParser)
			}
			r := isBuildPlatformDefault(tc.platform, mockParser)
			assert.Equal(t, r, tc.want, tc.name)
		})
	}
}
