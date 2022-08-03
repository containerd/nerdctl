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

/*
   Portions from https://github.com/docker/cli/blob/v20.10.9/cli/command/image/build/context.go
   Copyright (C) Docker authors.
   Licensed under the Apache License, Version 2.0
   NOTICE: https://github.com/docker/cli/blob/v20.10.9/NOTICE
*/

package buildkitutil

import (
	"reflect"
	"testing"
)

func TestParseBuildctlPruneOutput(t *testing.T) {
	type args struct {
		out []byte
	}
	tests := []struct {
		name    string
		args    args
		want    *BuildctlPruneOutput
		wantErr bool
	}{
		{
			name: "builtctl prune frees several spaces",
			args: args{out: []byte(`ID									RECLAIMABLE	SIZE	LAST ACCESSED
4kaw6aqapf3qvmqj3jskct89r                                              	true       	0B	
i0ys2vka15idnu952zc3le36o*                                             	true       	0B	
zn1cxnxqyxa18mzi5u2bnnqqk*                                             	true       	4.10kB	
x43clde10rasyy42vvzwo5w1r                                              	true       	156B	
kt007lnewdnssgukycqux8rcc                                              	true       	0B	
th54xgnz12r1rsgaoy55pfla1                                              	true       	0B	
66b6kbtv4iul87bzibzxwgo51                                              	true       	0B	
nlanvxjtqluiipuwva3uwfwvk                                              	true       	0B	
yh3oha6crdq9y34lmi2jshwhb                                              	true       	0B	
hai2h463u7o8xuybj0339e139                                              	true       	0B	
Total:	4.25kB
`)},
			want: &BuildctlPruneOutput{
				TotalSize: "584.31MB",
				Rows: []BuildctlPruneOutputRow{
					{
						ID:           "4kaw6aqapf3qvmqj3jskct89r",
						Reclaimable:  "true",
						Size:         "0B",
						LastAccessed: "",
					},
					{
						ID:           "i0ys2vka15idnu952zc3le36o*",
						Reclaimable:  "true",
						Size:         "0B",
						LastAccessed: "",
					},
					{
						ID:           "zn1cxnxqyxa18mzi5u2bnnqqk*",
						Reclaimable:  "true",
						Size:         "4.10kB",
						LastAccessed: "",
					},
					{
						ID:           "x43clde10rasyy42vvzwo5w1r",
						Reclaimable:  "true",
						Size:         "156B",
						LastAccessed: "",
					},
					{
						ID:           "kt007lnewdnssgukycqux8rcc",
						Reclaimable:  "true",
						Size:         "0B",
						LastAccessed: "",
					},
					{
						ID:           "th54xgnz12r1rsgaoy55pfla1",
						Reclaimable:  "true",
						Size:         "0B",
						LastAccessed: "",
					},
					{
						ID:           "66b6kbtv4iul87bzibzxwgo51",
						Reclaimable:  "true",
						Size:         "0B",
						LastAccessed: "",
					},
					{
						ID:           "nlanvxjtqluiipuwva3uwfwvk",
						Reclaimable:  "true",
						Size:         "0B",
						LastAccessed: "",
					},
					{
						ID:           "yh3oha6crdq9y34lmi2jshwhb",
						Reclaimable:  "true",
						Size:         "0B",
						LastAccessed: "",
					},
					{
						ID:           "hai2h463u7o8xuybj0339e139",
						Reclaimable:  "true",
						Size:         "0B",
						LastAccessed: "",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "buildctl prune frees no spaces",
			args: args{
				out: []byte(`Total:	0B
`),
			},
			want: &BuildctlPruneOutput{
				TotalSize: "0B",
				Rows:      nil,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBuildctlPruneTableOutput(tt.args.out)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBuildctlPruneTableOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseBuildctlPruneTableOutput() got = %v, want %v", got, tt.want)
			}
		})
	}
}
