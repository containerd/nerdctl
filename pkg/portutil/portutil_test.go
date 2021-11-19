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

package portutil

import (
	"reflect"
	"testing"

	gocni "github.com/containerd/go-cni"
)

func TestParseFlagP(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		want    []gocni.PortMapping
		wantErr bool
	}{
		{
			name: "normal",
			args: args{
				s: "127.0.0.1:3000:8080/tcp",
			},
			want: []gocni.PortMapping{
				{
					HostPort:      3000,
					ContainerPort: 8080,
					Protocol:      "tcp",
					HostIP:        "127.0.0.1",
				},
			},
			wantErr: false,
		},
		{
			name: "with port range",
			args: args{
				s: "127.0.0.1:3000-3001:8080-8081/tcp",
			},
			want: []gocni.PortMapping{
				{
					HostPort:      3000,
					ContainerPort: 8080,
					Protocol:      "tcp",
					HostIP:        "127.0.0.1",
				},
				{
					HostPort:      3001,
					ContainerPort: 8081,
					Protocol:      "tcp",
					HostIP:        "127.0.0.1",
				},
			},
			wantErr: false,
		},
		{
			name: "with wrong port range",
			args: args{
				s: "127.0.0.1:3000-3001:8080-8082/tcp",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "without host ip",
			args: args{
				s: "3000:8080/tcp",
			},
			want: []gocni.PortMapping{
				{
					HostPort:      3000,
					ContainerPort: 8080,
					Protocol:      "tcp",
					HostIP:        "0.0.0.0",
				},
			},
			wantErr: false,
		},
		{
			name: "without protocol",
			args: args{
				s: "3000:8080",
			},
			want: []gocni.PortMapping{
				{
					HostPort:      3000,
					ContainerPort: 8080,
					Protocol:      "tcp",
					HostIP:        "0.0.0.0",
				},
			},
			wantErr: false,
		},
		{
			name: "with protocol udp",
			args: args{
				s: "3000:8080/udp",
			},
			want: []gocni.PortMapping{
				{
					HostPort:      3000,
					ContainerPort: 8080,
					Protocol:      "udp",
					HostIP:        "0.0.0.0",
				},
			},
			wantErr: false,
		},
		{
			name: "with protocol udp",
			args: args{
				s: "3000:8080/sctp",
			},
			want: []gocni.PortMapping{
				{
					HostPort:      3000,
					ContainerPort: 8080,
					Protocol:      "sctp",
					HostIP:        "0.0.0.0",
				},
			},
			wantErr: false,
		},
		{
			name: "with invalid protocol",
			args: args{
				s: "3000:8080/invalid",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "without colon",
			args: args{
				s: "3000",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "multiple colon",
			args: args{
				s: "127.0.0.1:3000:0.0.0.0:8080",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "multiple slash",
			args: args{
				s: "127.0.0.1:3000:8080/tcp/",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid ip",
			args: args{
				s: "127.0.0.256:3000:8080/tcp",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "large port",
			args: args{
				s: "3000:65536",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "blank",
			args: args{
				s: "",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseFlagP(tt.args.s)
			t.Log(err)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFlagP() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFlagP() = %v, want %v", got, tt.want)
			}
		})
	}
}
