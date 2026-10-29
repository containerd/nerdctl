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

package container

import (
	"syscall"
	"testing"

	"gotest.tools/v3/assert"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/runtime/restart"

	"github.com/containerd/nerdctl/v2/pkg/labels"
)

func TestShouldUpdateExplicitlyStopped(t *testing.T) {
	tests := []struct {
		name            string
		sig             syscall.Signal
		containerLabels map[string]string
		expected        bool
		expectedErr     bool
	}{
		{
			name:            "SIGKILL always marks explicitly stopped without labels",
			sig:             syscall.SIGKILL,
			containerLabels: map[string]string{},
			expected:        true,
		},
		{
			name: "SIGKILL always marks explicitly stopped with custom stop signal and restart always",
			sig:  syscall.SIGKILL,
			containerLabels: map[string]string{
				labels.StopSignal:   "SIGUSR2",
				restart.PolicyLabel: "always",
			},
			expected: true,
		},
		{
			name: "SIGKILL always marks explicitly stopped with custom stop signal and restart unless-stopped",
			sig:  syscall.SIGKILL,
			containerLabels: map[string]string{
				labels.StopSignal:   "SIGUSR2",
				restart.PolicyLabel: "unless-stopped",
			},
			expected: true,
		},
		{
			name: "when no stop signal is configured, SIGHUP marks explicitly stopped for always",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				restart.PolicyLabel: "always",
			},
			expected: true,
		},
		{
			name: "when no stop signal is configured, SIGHUP marks explicitly stopped for unless-stopped",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				restart.PolicyLabel: "unless-stopped",
			},
			expected: true,
		},
		{
			name: "when no stop signal is configured, SIGTERM marks explicitly stopped for always",
			sig:  syscall.SIGTERM,
			containerLabels: map[string]string{
				restart.PolicyLabel: "always",
			},
			expected: true,
		},
		{
			name: "containerd.StopSignalLabel set to default SIGTERM without labels.StopSignal counts as unconfigured",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				containerd.StopSignalLabel: "SIGTERM",
				restart.PolicyLabel:        "always",
			},
			expected: true,
		},
		{
			name: "explicitly configured stop signal matches sent signal for always",
			sig:  syscall.SIGUSR2,
			containerLabels: map[string]string{
				labels.StopSignal:   "SIGUSR2",
				restart.PolicyLabel: "always",
			},
			expected: true,
		},
		{
			name: "explicitly configured stop signal does not match sent signal for always",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				labels.StopSignal:   "SIGUSR2",
				restart.PolicyLabel: "always",
			},
			expected: false,
		},
		{
			name: "explicitly configured stop signal does not match sent signal for on-failure",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				labels.StopSignal:   "SIGUSR2",
				restart.PolicyLabel: "on-failure:3",
			},
			expected: false,
		},
		{
			name: "explicitly configured stop signal does not match sent signal for restart policy no",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				labels.StopSignal:   "SIGUSR2",
				restart.PolicyLabel: "no",
			},
			expected: false,
		},
		{
			name: "explicitly configured stop signal does not match sent signal for unless-stopped marks explicitly stopped",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				labels.StopSignal:   "SIGUSR2",
				restart.PolicyLabel: "unless-stopped",
			},
			expected: true,
		},
		{
			name: "explicitly configured stop signal does not match sent signal using labels.RestartPolicy",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				labels.StopSignal:    "SIGUSR2",
				labels.RestartPolicy: "unless-stopped",
			},
			expected: true,
		},
		{
			name: "containerd.StopSignalLabel with non-SIGTERM signal matches sent signal",
			sig:  syscall.SIGQUIT,
			containerLabels: map[string]string{
				containerd.StopSignalLabel: "SIGQUIT",
				restart.PolicyLabel:        "always",
			},
			expected: true,
		},
		{
			name: "containerd.StopSignalLabel with non-SIGTERM signal does not match sent signal for always",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				containerd.StopSignalLabel: "SIGQUIT",
				restart.PolicyLabel:        "always",
			},
			expected: false,
		},
		{
			name: "containerd.StopSignalLabel with non-SIGTERM signal does not match sent signal for unless-stopped",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				containerd.StopSignalLabel: "SIGQUIT",
				restart.PolicyLabel:        "unless-stopped",
			},
			expected: true,
		},
		{
			name: "explicitly configured labels.StopSignal SIGTERM matches SIGTERM",
			sig:  syscall.SIGTERM,
			containerLabels: map[string]string{
				labels.StopSignal:   "SIGTERM",
				restart.PolicyLabel: "always",
			},
			expected: true,
		},
		{
			name: "explicitly configured labels.StopSignal SIGTERM does not match SIGHUP for always",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				labels.StopSignal:   "SIGTERM",
				restart.PolicyLabel: "always",
			},
			expected: false,
		},
		{
			name: "explicitly configured labels.StopSignal SIGTERM does not match SIGHUP for unless-stopped",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				labels.StopSignal:   "SIGTERM",
				restart.PolicyLabel: "unless-stopped",
			},
			expected: true,
		},
		{
			name: "unparsable stop signal returns error",
			sig:  syscall.SIGHUP,
			containerLabels: map[string]string{
				labels.StopSignal: "INVALID_SIGNAL",
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ShouldUpdateExplicitlyStopped(tt.sig, tt.containerLabels)
			if tt.expectedErr {
				assert.Assert(t, err != nil, "expected error, got nil")
				return
			}
			assert.NilError(t, err)
			assert.Equal(t, got, tt.expected)
		})
	}
}
