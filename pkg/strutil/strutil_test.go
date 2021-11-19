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

package strutil

import (
	"reflect"
	"testing"

	"gotest.tools/v3/assert"
)

func TestDedupeStrSlice(t *testing.T) {
	assert.DeepEqual(t,
		[]string{"apple", "banana", "chocolate"},
		DedupeStrSlice([]string{"apple", "banana", "apple", "chocolate"}))

	assert.DeepEqual(t,
		[]string{"apple", "banana", "chocolate"},
		DedupeStrSlice([]string{"apple", "apple", "banana", "chocolate", "apple"}))

}

func TestTrimStrSliceRight(t *testing.T) {
	assert.DeepEqual(t,
		[]string{"foo", "bar", "baz"},
		TrimStrSliceRight([]string{"foo", "bar", "baz", "qux", "quux"}, []string{"qux", "quux"}))
	assert.DeepEqual(t,
		[]string{"foo", "bar", "baz", "qux", "quux"},
		TrimStrSliceRight([]string{"foo", "bar", "baz", "qux", "quux"}, []string{"bar", "baz"}))
	assert.DeepEqual(t,
		[]string{},
		TrimStrSliceRight([]string{"foo", "bar", "baz", "qux", "quux"}, []string{"foo", "bar", "baz", "qux", "quux"}))
	assert.DeepEqual(t,
		[]string{"foo", "bar", "baz", "qux", "quux"},
		TrimStrSliceRight([]string{"foo", "bar", "baz", "qux", "quux"}, []string{}))
	assert.DeepEqual(t,
		[]string{"foo", "bar", "baz", "qux", "quux"},
		TrimStrSliceRight([]string{"foo", "bar", "baz", "qux", "quux"}, []string{"aaa"}))
	assert.DeepEqual(t,
		[]string{"foo", "bar", "baz", "qux"},
		TrimStrSliceRight([]string{"foo", "bar", "baz", "qux", "quux"}, []string{"quux"}))

}

func TestReverseStrSlice(t *testing.T) {
	assert.DeepEqual(t,
		[]string{"foo", "bar", "baz"},
		ReverseStrSlice([]string{"baz", "bar", "foo"}))
}

func TestParseBoolOrAuto(t *testing.T) {
	var xtrue = true
	var xfalse = false

	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		want    *bool
		wantErr bool
	}{
		{
			name: "normal-1",
			args: args{
				s: "true",
			},
			want:    &xtrue,
			wantErr: false,
		},
		{
			name: "normal-2",
			args: args{
				s: "false",
			},
			want:    &xfalse,
			wantErr: false,
		},
		{
			name: "blank",
			args: args{
				s: "",
			},
			want:    nil,
			wantErr: false,
		},
		{
			name: "auto",
			args: args{
				s: "auto",
			},
			want:    nil,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseBoolOrAuto(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseBoolOrAuto() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.want != nil {
				if *got != *tt.want {
					t.Errorf("ParseBoolOrAuto() = %v, want %v", got, tt.want)
				}
			} else if got != nil {
				t.Errorf("ParseBoolOrAuto() = %v, want %v", got, nil)
			}
		})
	}
}

func TestConvertKVStringsToMap(t *testing.T) {
	type args struct {
		values []string
	}
	tests := []struct {
		name string
		args args
		want map[string]string
	}{
		{
			name: "normal",
			args: args{
				values: []string{"foo=bar", "baz=qux"},
			},
			want: map[string]string{
				"foo": "bar",
				"baz": "qux",
			},
		},
		{
			name: "normal-1",
			args: args{
				values: []string{"foo"},
			},
			want: map[string]string{
				"foo": "",
			},
		},
		{
			name: "normal-2",
			args: args{
				values: []string{"foo=bar=baz"},
			},
			want: map[string]string{
				"foo": "bar=baz",
			},
		},
		{
			name: "empty",
			args: args{
				values: []string{},
			},
			want: map[string]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ConvertKVStringsToMap(tt.args.values); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ConvertKVStringsToMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInStringSlice(t *testing.T) {
	type args struct {
		ss  []string
		str string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "normal",
			args: args{
				ss:  []string{"foo", "bar", "baz"},
				str: "bar",
			},
			want: true,
		},
		{
			name: "normal-1",
			args: args{
				ss:  []string{"foo", "bar", "baz"},
				str: "qux",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InStringSlice(tt.args.ss, tt.args.str); got != tt.want {
				t.Errorf("InStringSlice() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCSVMap(t *testing.T) {
	type args struct {
		s string
	}
	tests := []struct {
		name    string
		args    args
		want    map[string]string
		wantErr bool
	}{
		{
			name: "normal",
			args: args{
				s: "foo=x,bar=y,baz=z,qux",
			},
			want: map[string]string{
				"foo": "x",
				"bar": "y",
				"baz": "z",
				"qux": "",
			},
			wantErr: false,
		},
		{
			name: "normal-1",
			args: args{
				s: "\"foo=x,bar=y\",baz=z,qux",
			},
			want: map[string]string{
				"foo": "x,bar=y",
				"baz": "z",
				"qux": "",
			},
			wantErr: false,
		},
		{
			name: "invalid",
			args: args{
				s: "sssssss\nsss",
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "invalid-1",
			args: args{
				s: "sssssss\n\"\nsss",
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseCSVMap(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseCSVMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseCSVMap() = %v, want %v", got, tt.want)
			}
		})
	}
}
