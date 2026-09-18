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

package image

import (
	"testing"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"gotest.tools/v3/assert"
)

func TestApplyChanges(t *testing.T) {
	testCases := []struct {
		name    string
		changes []string
		want    ocispec.ImageConfig
	}{
		{
			name:    "CMD exec form",
			changes: []string{`CMD ["echo","hi"]`},
			want:    ocispec.ImageConfig{Cmd: []string{"echo", "hi"}},
		},
		{
			name:    "CMD shell form wraps in sh -c",
			changes: []string{"CMD echo hi there"},
			want:    ocispec.ImageConfig{Cmd: []string{"/bin/sh", "-c", "echo hi there"}},
		},
		{
			name:    "CMD starting with bracket falls back to shell form",
			changes: []string{"CMD [ -f /healthy ]"},
			want:    ocispec.ImageConfig{Cmd: []string{"/bin/sh", "-c", "[ -f /healthy ]"}},
		},
		{
			name:    "ENTRYPOINT exec form",
			changes: []string{`ENTRYPOINT ["/app","--flag"]`},
			want:    ocispec.ImageConfig{Entrypoint: []string{"/app", "--flag"}},
		},
		{
			name:    "ENV key=value pairs with quoted value",
			changes: []string{`ENV FOO=bar BAZ="q u x"`},
			want:    ocispec.ImageConfig{Env: []string{"FOO=bar", "BAZ=q u x"}},
		},
		{
			name:    "ENV legacy key value form",
			changes: []string{"ENV FOO bar baz"},
			want:    ocispec.ImageConfig{Env: []string{"FOO=bar baz"}},
		},
		{
			name:    "ENV later change overrides same key",
			changes: []string{"ENV FOO=a", "ENV FOO=b"},
			want:    ocispec.ImageConfig{Env: []string{"FOO=b"}},
		},
		{
			name:    "LABEL pairs",
			changes: []string{`LABEL a=1 b="two words"`},
			want:    ocispec.ImageConfig{Labels: map[string]string{"a": "1", "b": "two words"}},
		},
		{
			name:    "EXPOSE default tcp and explicit udp",
			changes: []string{"EXPOSE 80 53/udp"},
			want:    ocispec.ImageConfig{ExposedPorts: map[string]struct{}{"80/tcp": {}, "53/udp": {}}},
		},
		{
			name:    "EXPOSE port range expands to each port",
			changes: []string{"EXPOSE 8080-8082"},
			want:    ocispec.ImageConfig{ExposedPorts: map[string]struct{}{"8080/tcp": {}, "8081/tcp": {}, "8082/tcp": {}}},
		},
		{
			name:    "EXPOSE protocol is case-insensitive",
			changes: []string{"EXPOSE 80/TCP"},
			want:    ocispec.ImageConfig{ExposedPorts: map[string]struct{}{"80/tcp": {}}},
		},
		{
			name:    "VOLUME json and shell forms",
			changes: []string{`VOLUME ["/data"]`, "VOLUME /a /b"},
			want:    ocispec.ImageConfig{Volumes: map[string]struct{}{"/data": {}, "/a": {}, "/b": {}}},
		},
		{
			name:    "USER WORKDIR STOPSIGNAL",
			changes: []string{"USER nobody:nogroup", "WORKDIR /srv", "STOPSIGNAL SIGTERM"},
			want:    ocispec.ImageConfig{User: "nobody:nogroup", WorkingDir: "/srv", StopSignal: "SIGTERM"},
		},
		{
			name:    "instruction keyword is case-insensitive",
			changes: []string{"workdir /w"},
			want:    ocispec.ImageConfig{WorkingDir: "/w"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg ocispec.ImageConfig
			err := applyChanges(&cfg, tc.changes)
			assert.NilError(t, err)
			assert.DeepEqual(t, tc.want, cfg)
		})
	}
}

func TestApplyChangesErrors(t *testing.T) {
	testCases := []struct {
		name   string
		change string
		errSub string
	}{
		{"unknown instruction", "RUN echo hi", "unknown instruction"},
		{"healthcheck unsupported", "HEALTHCHECK CMD true", "not supported by import"},
		{"onbuild unsupported", "ONBUILD RUN true", "not supported by import"},
		{"shell unsupported", `SHELL ["/bin/bash","-c"]`, "not supported by import"},
		{"expose non-numeric port", "EXPOSE http", "invalid EXPOSE port"},
		{"expose bad proto", "EXPOSE 80/icmp", "invalid EXPOSE protocol"},
		{"expose reversed range", "EXPOSE 90-80", "invalid EXPOSE port range"},
		{"label without value", "LABEL a=1 b", "expected key=value"},
		{"env legacy without a value", "ENV LONELYKEY", "two arguments"},
		{"env unterminated quote", `ENV A="oops`, "unterminated quote"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var cfg ocispec.ImageConfig
			err := applyChanges(&cfg, []string{tc.change})
			assert.ErrorContains(t, err, tc.errSub)
		})
	}
}
