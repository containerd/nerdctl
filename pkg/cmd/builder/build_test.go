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
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	specs "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"
)

// fakePlatformParser is a hand-rolled test double for PlatformParser.
// Tests assign the function fields to control behavior.
type fakePlatformParser struct {
	ParseFunc       func(platform string) (specs.Platform, error)
	DefaultSpecFunc func() specs.Platform
}

func (f *fakePlatformParser) Parse(platform string) (specs.Platform, error) {
	if f.ParseFunc == nil {
		return specs.Platform{}, nil
	}
	return f.ParseFunc(platform)
}

func (f *fakePlatformParser) DefaultSpec() specs.Platform {
	if f.DefaultSpecFunc == nil {
		return specs.Platform{}
	}
	return f.DefaultSpecFunc()
}

func TestIsMatchingRuntimePlatform(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		parser *fakePlatformParser
		want   bool
	}{
		{
			name: "Image is shareable when Runtime and build platform match for os, arch and variant",
			parser: &fakePlatformParser{
				ParseFunc: func(string) (specs.Platform, error) {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}, nil
				},
				DefaultSpecFunc: func() specs.Platform {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}
				},
			},
			want: true,
		},
		{
			name: "Image is shareable when Runtime and build platform match for os, arch. Variant is not defined",
			parser: &fakePlatformParser{
				ParseFunc: func(string) (specs.Platform, error) {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: ""}, nil
				},
				DefaultSpecFunc: func() specs.Platform {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}
				},
			},
			want: true,
		},
		{
			name: "Image is not shareable when Runtime and build platform donot math OS",
			parser: &fakePlatformParser{
				ParseFunc: func(string) (specs.Platform, error) {
					return specs.Platform{OS: "OS", Architecture: "mockArch", Variant: ""}, nil
				},
				DefaultSpecFunc: func() specs.Platform {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}
				},
			},
			want: false,
		},
		{
			name: "Image is not shareable when Runtime and build platform donot math Arch",
			parser: &fakePlatformParser{
				ParseFunc: func(string) (specs.Platform, error) {
					return specs.Platform{OS: "mockOS", Architecture: "Arch", Variant: ""}, nil
				},
				DefaultSpecFunc: func() specs.Platform {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}
				},
			},
			want: false,
		},
		{
			name: "Image is not shareable when Runtime and build platform donot math Variant",
			parser: &fakePlatformParser{
				ParseFunc: func(string) (specs.Platform, error) {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "Variant"}, nil
				},
				DefaultSpecFunc: func() specs.Platform {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}
				},
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := isMatchingRuntimePlatform("test", tc.parser)
			assert.Equal(t, r, tc.want, tc.name)
		})
	}
}

func TestIsBuildPlatformDefault(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		parser   *fakePlatformParser
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
			parser: &fakePlatformParser{
				ParseFunc: func(string) (specs.Platform, error) {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}, nil
				},
				DefaultSpecFunc: func() specs.Platform {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}
				},
			},
			want: true,
		},
		{
			name:     "Image is not shareable when Runtime build platform dont match",
			platform: []string{"test"},
			parser: &fakePlatformParser{
				ParseFunc: func(string) (specs.Platform, error) {
					return specs.Platform{OS: "OS", Architecture: "mockArch", Variant: "mockVariant"}, nil
				},
				DefaultSpecFunc: func() specs.Platform {
					return specs.Platform{OS: "mockOS", Architecture: "mockArch", Variant: "mockVariant"}
				},
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
			parser := tc.parser
			if parser == nil {
				parser = &fakePlatformParser{}
			}
			r := isBuildPlatformDefault(tc.platform, parser)
			assert.Equal(t, r, tc.want, tc.name)
		})
	}
}

func TestParseBuildctlArgsForOCILayout(t *testing.T) {
	tests := []struct {
		name          string
		ociLayoutName string
		ociLayoutPath string
		expectedArgs  []string
		errorIsNil    bool
		expectedErr   string
	}{
		{
			name:          "PrefixNotFoundError",
			ociLayoutName: "unit-test",
			ociLayoutPath: "/tmp/oci-layout/",
			expectedArgs:  []string{},
			expectedErr:   ErrOCILayoutPrefixNotFound.Error(),
		},
		{
			name:          "DirectoryNotFoundError",
			ociLayoutName: "unit-test",
			ociLayoutPath: "oci-layout:///tmp/oci-layout",
			expectedArgs:  []string{},
			expectedErr:   "open /tmp/oci-layout/index.json: no such file or directory",
		},
	}

	if runtime.GOOS == "windows" {
		abspath, err := filepath.Abs("/tmp/oci-layout")
		assert.NilError(t, err)
		tests[1].expectedErr = fmt.Sprintf(
			"open %s\\index.json: The system cannot find the path specified.",
			abspath,
		)
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args, err := parseBuildContextFromOCILayout(test.ociLayoutName, test.ociLayoutPath)
			if test.errorIsNil {
				assert.NilError(t, err)
			} else {
				assert.Error(t, err, test.expectedErr)
			}
			assert.Equal(t, len(args), len(test.expectedArgs))
			assert.DeepEqual(t, args, test.expectedArgs)
		})
	}
}

func TestGetEffectiveSourcePolicyFile(t *testing.T) {
	// Cannot use t.Parallel() since subtests modify environment variables

	tests := []struct {
		name        string
		optionValue string
		envValue    string
		expected    string
	}{
		{
			name:        "option value takes precedence over env var",
			optionValue: "/path/from/flag.json",
			envValue:    "/path/from/env.json",
			expected:    "/path/from/flag.json",
		},
		{
			name:        "env var is used when option is empty",
			optionValue: "",
			envValue:    "/path/from/env.json",
			expected:    "/path/from/env.json",
		},
		{
			name:        "empty when both are unset",
			optionValue: "",
			envValue:    "",
			expected:    "",
		},
		{
			name:        "option value used when env var is empty",
			optionValue: "/path/from/flag.json",
			envValue:    "",
			expected:    "/path/from/flag.json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Set up the environment variable for this test
			t.Setenv("EXPERIMENTAL_BUILDKIT_SOURCE_POLICY", tc.envValue)

			result := GetEffectiveSourcePolicyFile(tc.optionValue)
			assert.Equal(t, result, tc.expected)
		})
	}
}
