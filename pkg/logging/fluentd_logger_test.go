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

package logging

import (
	"reflect"
	"testing"

	"github.com/fluent/fluent-logger-golang/fluent"
)

func TestParseAddress(t *testing.T) {
	type args struct {
		address string
	}
	tests := []struct {
		name    string
		args    args
		want    *fluentdLocation
		wantErr bool
	}{
		{name: "empty", args: args{address: ""}, want: &fluentdLocation{protocol: "tcp", host: "127.0.0.1", port: 24224}, wantErr: false},
		{name: "unix", args: args{address: "unix:///var/run/fluentd/fluentd.sock"}, want: &fluentdLocation{protocol: "unix", path: "/var/run/fluentd/fluentd.sock"}, wantErr: false},
		{name: "tcp", args: args{address: "tcp://127.0.0.1:24224"}, want: &fluentdLocation{protocol: "tcp", host: "127.0.0.1", port: 24224}, wantErr: false},
		{name: "tcpWithPath", args: args{address: "tcp://127.0.0.1:24224/1234"}, want: nil, wantErr: true},
		{name: "unixWithEmpty", args: args{address: "unix://"}, want: nil, wantErr: true},
		{name: "invalidPath", args: args{address: "://asd123"}, want: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAddress(tt.args.address)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseAddress() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseAddress() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateFluentdLoggerOpts(t *testing.T) {
	type args struct {
		config map[string]string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		// TODO: Add test cases.
		{name: "empty", args: args{config: map[string]string{}}, wantErr: false},
		{name: "invalid", args: args{config: map[string]string{"foo": "bar"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateFluentdLoggerOpts(tt.args.config); (err != nil) != tt.wantErr {
				t.Errorf("ValidateFluentdLoggerOpts() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseFluentdConfig(t *testing.T) {
	type args struct {
		config map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    fluent.Config
		wantErr bool
	}{
		{"DefaultLocation", args{
			config: map[string]string{}},
			fluent.Config{
				FluentPort:             defaultPort,
				FluentHost:             defaultHost,
				FluentNetwork:          defaultProtocol,
				FluentSocketPath:       "",
				BufferLimit:            defaultBufferLimit,
				RetryWait:              int(defaultRetryWait),
				MaxRetry:               defaultMaxRetries,
				Async:                  false,
				AsyncReconnectInterval: 0,
				SubSecondPrecision:     false,
				RequestAck:             false}, false},
		{"InputLocation", args{
			config: map[string]string{
				fluentAddress: "tcp://127.0.0.1:123",
			}},
			fluent.Config{
				FluentPort:             123,
				FluentHost:             "127.0.0.1",
				FluentNetwork:          defaultProtocol,
				FluentSocketPath:       "",
				BufferLimit:            defaultBufferLimit,
				RetryWait:              int(defaultRetryWait),
				MaxRetry:               defaultMaxRetries,
				Async:                  false,
				AsyncReconnectInterval: 0,
				SubSecondPrecision:     false,
				RequestAck:             false}, false},
		{"InvalidLocation", args{config: map[string]string{fluentAddress: "://asd123"}}, fluent.Config{}, true},
		{"InputAsyncOption", args{
			config: map[string]string{
				fluentdAsync: "true",
			}},
			fluent.Config{
				FluentPort:             defaultPort,
				FluentHost:             defaultHost,
				FluentNetwork:          defaultProtocol,
				FluentSocketPath:       "",
				BufferLimit:            defaultBufferLimit,
				RetryWait:              int(defaultRetryWait),
				MaxRetry:               defaultMaxRetries,
				Async:                  true,
				AsyncReconnectInterval: 0,
				SubSecondPrecision:     false,
				RequestAck:             false}, false},
		{"InputAsyncReconnectOption", args{
			config: map[string]string{
				fluentdAsyncReconnectInterval: "100ms",
			}},
			fluent.Config{
				FluentPort:             defaultPort,
				FluentHost:             defaultHost,
				FluentNetwork:          defaultProtocol,
				FluentSocketPath:       "",
				BufferLimit:            defaultBufferLimit,
				RetryWait:              int(defaultRetryWait),
				MaxRetry:               defaultMaxRetries,
				Async:                  false,
				AsyncReconnectInterval: 100,
				SubSecondPrecision:     false,
				RequestAck:             false}, false},
		{"InputBufferLimitOption", args{
			config: map[string]string{
				fluentdBufferLimit: "1000",
			}},
			fluent.Config{
				FluentPort:             defaultPort,
				FluentHost:             defaultHost,
				FluentNetwork:          defaultProtocol,
				FluentSocketPath:       "",
				BufferLimit:            1000,
				RetryWait:              int(defaultRetryWait),
				MaxRetry:               defaultMaxRetries,
				Async:                  false,
				AsyncReconnectInterval: 0,
				SubSecondPrecision:     false,
				RequestAck:             false}, false},
		{"InputRetryWaitOption", args{
			config: map[string]string{
				fluentdRetryWait: "10s",
			}},
			fluent.Config{
				FluentPort:             defaultPort,
				FluentHost:             defaultHost,
				FluentNetwork:          defaultProtocol,
				FluentSocketPath:       "",
				BufferLimit:            defaultBufferLimit,
				RetryWait:              10000,
				MaxRetry:               defaultMaxRetries,
				Async:                  false,
				AsyncReconnectInterval: 0,
				SubSecondPrecision:     false,
				RequestAck:             false}, false},
		{"InputMaxRetriesOption", args{
			config: map[string]string{
				fluentdMaxRetries: "100",
			}},
			fluent.Config{
				FluentPort:             defaultPort,
				FluentHost:             defaultHost,
				FluentNetwork:          defaultProtocol,
				FluentSocketPath:       "",
				BufferLimit:            defaultBufferLimit,
				RetryWait:              int(defaultRetryWait),
				MaxRetry:               100,
				Async:                  false,
				AsyncReconnectInterval: 0,
				SubSecondPrecision:     false,
				RequestAck:             false}, false},
		{"InputSubSecondPrecision", args{
			config: map[string]string{
				fluentdSubSecondPrecision: "true",
			}},
			fluent.Config{
				FluentPort:             defaultPort,
				FluentHost:             defaultHost,
				FluentNetwork:          defaultProtocol,
				FluentSocketPath:       "",
				BufferLimit:            defaultBufferLimit,
				RetryWait:              int(defaultRetryWait),
				MaxRetry:               defaultMaxRetries,
				Async:                  false,
				AsyncReconnectInterval: 0,
				SubSecondPrecision:     true,
				RequestAck:             false}, false},
		{"InputRequestAck", args{
			config: map[string]string{
				fluentRequestAck: "true",
			}},
			fluent.Config{
				FluentPort:             defaultPort,
				FluentHost:             defaultHost,
				FluentNetwork:          defaultProtocol,
				FluentSocketPath:       "",
				BufferLimit:            defaultBufferLimit,
				RetryWait:              int(defaultRetryWait),
				MaxRetry:               defaultMaxRetries,
				Async:                  false,
				AsyncReconnectInterval: 0,
				SubSecondPrecision:     false,
				RequestAck:             true}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFluentdConfig(tt.args.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFluentdConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFluentdConfig() got = %v, want %v", got, tt.want)
			}
		})
	}
}
