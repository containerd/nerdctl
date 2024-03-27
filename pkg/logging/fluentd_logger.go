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
	"fmt"
	"math"
	"net/url"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containerd/containerd/v2/core/runtime/v2/logging"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/v2/pkg/strutil"
	"github.com/fluent/fluent-logger-golang/fluent"
)

type FluentdLogger struct {
	Opts         map[string]string
	fluentClient *fluent.Fluent
	config       *logging.Config
}

const (
	fluentAddress                 = "fluentd-address"
	fluentdAsync                  = "fluentd-async"
	fluentdBufferLimit            = "fluentd-buffer-limit"
	fluentdRetryWait              = "fluentd-retry-wait"
	fluentdMaxRetries             = "fluentd-max-retries"
	fluentdSubSecondPrecision     = "fluentd-sub-second-precision"
	fluentdAsyncReconnectInterval = "fluentd-async-reconnect-interval"
	fluentRequestAck              = "fluentd-request-ack"
)

var FluentdLogOpts = []string{
	fluentAddress,
	fluentdAsync,
	fluentdBufferLimit,
	fluentdRetryWait,
	fluentdMaxRetries,
	fluentdSubSecondPrecision,
	fluentdAsyncReconnectInterval,
	fluentRequestAck,
	Tag,
}

const (
	defaultBufferLimit = 1024 * 1024
	defaultHost        = "127.0.0.1"
	defaultPort        = 24224
	defaultProtocol    = "tcp"

	defaultMaxRetries = math.MaxInt32
	defaultRetryWait  = 1000 * time.Millisecond

	minReconnectInterval = 100 * time.Millisecond
	maxReconnectInterval = 10 * time.Second
)

func FluentdLogOptsValidate(logOptMap map[string]string) error {
	for key := range logOptMap {
		if !strutil.InStringSlice(FluentdLogOpts, key) {
			log.L.Warnf("log-opt %s is ignored for fluentd log driver", key)
		}
	}
	if _, ok := logOptMap[fluentAddress]; !ok {
		log.L.Warnf("%s is missing for fluentd log driver, the default value %s:%d will be used", fluentAddress, defaultHost, defaultPort)
	}
	return nil
}

type fluentdLocation struct {
	protocol string
	host     string
	port     int
	path     string
}

func (f *FluentdLogger) Init(dataStore, ns, id string) error {
	return nil
}

func (f *FluentdLogger) PreProcess(_ string, config *logging.Config) error {
	if runtime.GOOS == "windows" {
		// TODO: support fluentd on windows
		return fmt.Errorf("logging to fluentd is not supported on windows")
	}
	fluentConfig, err := parseFluentdConfig(f.Opts)
	if err != nil {
		return err
	}
	fluentClient, err := fluent.New(fluentConfig)
	if err != nil {
		return fmt.Errorf("failed to create fluent client: %w", err)
	}
	f.fluentClient = fluentClient
	f.config = config
	return nil
}
func (f *FluentdLogger) Process(stdout <-chan string, stderr <-chan string) error {
	var wg sync.WaitGroup
	wg.Add(2)
	fun := func(wg *sync.WaitGroup, dataChan <-chan string, id, namespace, source string) {
		defer wg.Done()
		metaData := map[string]string{
			"container_id": id,
			"namespace":    namespace,
			"source":       source,
		}
		for log := range dataChan {
			metaData["log"] = log
			f.fluentClient.PostWithTime(f.Opts[Tag], time.Now(), metaData)
		}
	}
	go fun(&wg, stdout, f.config.ID, f.config.Namespace, "stdout")
	go fun(&wg, stderr, f.config.ID, f.config.Namespace, "stderr")

	wg.Wait()
	return nil
}

func (f *FluentdLogger) PostProcess() error {
	defer f.fluentClient.Close()
	return nil
}

func parseAddress(address string) (*fluentdLocation, error) {
	if address == "" {
		return &fluentdLocation{
			protocol: defaultProtocol,
			host:     defaultHost,
			port:     defaultPort,
		}, nil
	}
	if !strings.Contains(address, "://") {
		address = defaultProtocol + "://" + address
	}
	tempURL, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	switch tempURL.Scheme {
	case "unix":
		if strings.TrimLeft(tempURL.Path, "/") == "" {
			return nil, fmt.Errorf("unix socket path must not be empty")
		}
		return &fluentdLocation{
			protocol: tempURL.Scheme,
			path:     tempURL.Path,
		}, nil
	case "tcp", "tls":
	// continue to process below
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", tempURL.Scheme)
	}
	if tempURL.Path != "" {
		return nil, fmt.Errorf("path is not supported: %s", tempURL.Path)
	}
	host := defaultHost
	port := defaultPort
	if h := tempURL.Hostname(); h != "" {
		host = h
	}
	if p := tempURL.Port(); p != "" {
		portNum, err := strconv.ParseUint(p, 10, 16)
		if err != nil {
			return nil, fmt.Errorf("error occurs %v,invalid port", err)
		}
		port = int(portNum)
	}
	return &fluentdLocation{
		protocol: tempURL.Scheme,
		host:     host,
		port:     port,
	}, nil
}

func ValidateFluentdLoggerOpts(config map[string]string) error {
	for key := range config {
		switch key {
		case Tag:
		case fluentdBufferLimit:
		case fluentdMaxRetries:
		case fluentdRetryWait:
		case fluentdSubSecondPrecision:
		case fluentdAsync:
		case fluentAddress:
		case fluentdAsyncReconnectInterval:
		case fluentRequestAck:
		// Accepted logger opts
		default:
			return fmt.Errorf("unknown log opt '%s' for fluentd log driver", key)
		}
	}
	return nil
}

func parseFluentdConfig(config map[string]string) (fluent.Config, error) {
	result := fluent.Config{}
	location, err := parseAddress(config[fluentAddress])
	if err != nil {
		return result, fmt.Errorf("error occurs %v,invalid fluentd address (%s)", err, config[fluentAddress])
	}
	bufferLimit := defaultBufferLimit
	if config[fluentdBufferLimit] != "" {
		bufferLimit, err = strconv.Atoi(config[fluentdBufferLimit])
		if err != nil {
			return result, fmt.Errorf("error occurs %v,invalid buffer limit (%s)", err, config[fluentdBufferLimit])
		}
	}
	retryWait := int(defaultRetryWait)
	if config[fluentdRetryWait] != "" {
		temp, err := time.ParseDuration(config[fluentdRetryWait])
		if err != nil {
			return result, fmt.Errorf("error occurs %v,invalid retry wait (%s)", err, config[fluentdRetryWait])
		}
		retryWait = int(temp.Milliseconds())
	}
	maxRetries := defaultMaxRetries
	if config[fluentdMaxRetries] != "" {
		maxRetries, err = strconv.Atoi(config[fluentdMaxRetries])
		if err != nil {
			return result, fmt.Errorf("error occurs %v,invalid max retries (%s)", err, config[fluentdMaxRetries])
		}
	}
	async := false
	if config[fluentdAsync] != "" {
		async, err = strconv.ParseBool(config[fluentdAsync])
		if err != nil {
			return result, fmt.Errorf("error occurs %v,invalid async (%s)", err, config[fluentdAsync])
		}
	}
	asyncReconnectInterval := 0
	if config[fluentdAsyncReconnectInterval] != "" {
		tempDuration, err := time.ParseDuration(config[fluentdAsyncReconnectInterval])
		if err != nil {
			return result, fmt.Errorf("error occurs %v,invalid async connect interval (%s)", err, config[fluentdAsyncReconnectInterval])
		}
		if tempDuration != 0 && (tempDuration < minReconnectInterval || tempDuration > maxReconnectInterval) {
			return result, fmt.Errorf("invalid async connect interval (%s), must be between %d and %d", config[fluentdAsyncReconnectInterval], minReconnectInterval.Milliseconds(), maxReconnectInterval.Milliseconds())
		}
		asyncReconnectInterval = int(tempDuration.Milliseconds())
	}
	subSecondPrecision := false
	if config[fluentdSubSecondPrecision] != "" {
		subSecondPrecision, err = strconv.ParseBool(config[fluentdSubSecondPrecision])
		if err != nil {
			return result, fmt.Errorf("error occurs %v,invalid sub second precision (%s)", err, config[fluentdSubSecondPrecision])
		}
	}
	requestAck := false
	if config[fluentRequestAck] != "" {
		requestAck, err = strconv.ParseBool(config[fluentRequestAck])
		if err != nil {
			return result, fmt.Errorf("error occurs %v,invalid request ack (%s)", err, config[fluentRequestAck])
		}
	}
	result = fluent.Config{
		FluentPort:             location.port,
		FluentHost:             location.host,
		FluentNetwork:          location.protocol,
		FluentSocketPath:       location.path,
		BufferLimit:            bufferLimit,
		RetryWait:              retryWait,
		MaxRetry:               maxRetries,
		Async:                  async,
		AsyncReconnectInterval: asyncReconnectInterval,
		SubSecondPrecision:     subSecondPrecision,
		RequestAck:             requestAck,
	}
	return result, nil
}
