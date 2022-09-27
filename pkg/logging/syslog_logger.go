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
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/docker/go-connections/tlsconfig"
	syslog "github.com/yuchanns/srslog"

	"github.com/containerd/containerd/runtime/v2/logging"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/sirupsen/logrus"
)

const (
	syslogAddress       = "syslog-address"
	syslogFacility      = "syslog-facility"
	syslogTLSCaCert     = "syslog-tls-ca-cert"
	syslogTLSCert       = "syslog-tls-cert"
	syslogTLSKey        = "syslog-tls-key"
	syslogTLSSkipVerify = "syslog-tls-skip-verify"
	syslogFormat        = "syslog-format"
)

var syslogOpts = []string{
	syslogAddress,
	syslogFacility,
	syslogTLSCaCert,
	syslogTLSCert,
	syslogTLSKey,
	syslogTLSSkipVerify,
	syslogFormat,
	Tag,
}

var syslogFacilities = map[string]syslog.Priority{
	"kern":     syslog.LOG_KERN,
	"user":     syslog.LOG_USER,
	"mail":     syslog.LOG_MAIL,
	"daemon":   syslog.LOG_DAEMON,
	"auth":     syslog.LOG_AUTH,
	"syslog":   syslog.LOG_SYSLOG,
	"lpr":      syslog.LOG_LPR,
	"news":     syslog.LOG_NEWS,
	"uucp":     syslog.LOG_UUCP,
	"cron":     syslog.LOG_CRON,
	"authpriv": syslog.LOG_AUTHPRIV,
	"ftp":      syslog.LOG_FTP,
	"local0":   syslog.LOG_LOCAL0,
	"local1":   syslog.LOG_LOCAL1,
	"local2":   syslog.LOG_LOCAL2,
	"local3":   syslog.LOG_LOCAL3,
	"local4":   syslog.LOG_LOCAL4,
	"local5":   syslog.LOG_LOCAL5,
	"local6":   syslog.LOG_LOCAL6,
	"local7":   syslog.LOG_LOCAL7,
}

const (
	syslogSecureProto = "tcp+tls"
	syslogDefaultPort = "514"

	syslogFormatRFC3164      = "rfc3164"
	syslogFormatRFC5424      = "rfc5424"
	syslogFormatRFC5424Micro = "rfc5424micro"
)

func SyslogOptsValidate(logOptMap map[string]string) error {
	for key := range logOptMap {
		if !strutil.InStringSlice(syslogOpts, key) {
			logrus.Warnf("log-opt %s is ignored for syslog log driver", key)
		}
	}
	proto, _, err := parseSyslogAddress(logOptMap[syslogAddress])
	if err != nil {
		return err
	}
	if _, err := parseSyslogFacility(logOptMap[syslogFacility]); err != nil {
		return err
	}
	if _, _, err := parseSyslogLogFormat(logOptMap[syslogFormat], proto); err != nil {
		return err
	}
	if proto == syslogSecureProto {
		if _, tlsErr := parseTLSConfig(logOptMap); tlsErr != nil {
			return tlsErr
		}
	}
	return nil
}

type SyslogLogger struct {
	Opts map[string]string
}

func (sy *SyslogLogger) Init(dataStore string, ns string, id string) error {
	return nil
}

func (sy *SyslogLogger) Process(dataStore string, config *logging.Config) error {
	logger, err := parseSyslog(config.ID, sy.Opts)
	if err != nil {
		return err
	}
	defer logger.Close()
	var wg sync.WaitGroup
	wg.Add(2)
	fn := func(r io.Reader, logFn func(msg string) error) {
		defer wg.Done()
		s := bufio.NewScanner(r)
		for s.Scan() {
			if s.Err() != nil {
				return
			}
			logFn(s.Text())
		}
	}
	go fn(config.Stdout, logger.Info)
	go fn(config.Stderr, logger.Err)
	wg.Wait()
	return nil
}

func parseSyslog(containerID string, config map[string]string) (*syslog.Writer, error) {
	tag := containerID[:12]
	if cfgTag, ok := config[Tag]; ok {
		tag = cfgTag
	}
	proto, address, err := parseSyslogAddress(config[syslogAddress])
	if err != nil {
		return nil, err
	}
	facility, err := parseSyslogFacility(config[syslogFacility])
	if err != nil {
		return nil, err
	}
	syslogFormatter, syslogFramer, err := parseSyslogLogFormat(config[syslogFormat], proto)
	if err != nil {
		return nil, err
	}
	var logger *syslog.Writer
	if proto == syslogSecureProto {
		tlsConfig, tlsErr := parseTLSConfig(config)
		if tlsErr != nil {
			return nil, tlsErr
		}
		logger, err = syslog.DialWithTLSConfig(proto, address, facility, tag, tlsConfig)
	} else {
		logger, err = syslog.Dial(proto, address, facility, tag)
	}

	if err != nil {
		return nil, err
	}

	logger.SetFormatter(syslogFormatter)
	logger.SetFramer(syslogFramer)

	return logger, nil
}

func parseSyslogAddress(address string) (string, string, error) {
	if address == "" {
		// Docker-compatible: fallback to `unix:///dev/log`,
		// `unix:///var/run/syslog` or `unix:///var/run/log`. We do nothing
		// with the empty address, just leave it here and the srslog will
		// handle the fallback.
		return "", "", nil
	}
	addr, err := url.Parse(address)
	if err != nil {
		return "", "", err
	}

	// unix and unixgram socket validation
	if addr.Scheme == "unix" || addr.Scheme == "unixgram" {
		if _, err := os.Stat(addr.Path); err != nil {
			return "", "", err
		}
		return addr.Scheme, addr.Path, nil
	}
	if addr.Scheme != "udp" && addr.Scheme != "tcp" && addr.Scheme != syslogSecureProto {
		return "", "", fmt.Errorf("unsupported scheme: '%s'", addr.Scheme)
	}

	// here we process tcp|udp
	host := addr.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		if !strings.Contains(err.Error(), "missing port in address") {
			return "", "", err
		}
		host = net.JoinHostPort(host, syslogDefaultPort)
	}

	return addr.Scheme, host, nil
}

func parseSyslogFacility(facility string) (syslog.Priority, error) {
	if facility == "" {
		return syslog.LOG_DAEMON, nil
	}

	if syslogFacility, valid := syslogFacilities[facility]; valid {
		return syslogFacility, nil
	}

	fInt, err := strconv.Atoi(facility)
	if err == nil && 0 <= fInt && fInt <= 23 {
		return syslog.Priority(fInt << 3), nil
	}

	return syslog.Priority(0), errors.New("invalid syslog facility")
}

func parseTLSConfig(cfg map[string]string) (*tls.Config, error) {
	_, skipVerify := cfg[syslogTLSSkipVerify]

	opts := tlsconfig.Options{
		CAFile:             cfg[syslogTLSCaCert],
		CertFile:           cfg[syslogTLSCert],
		KeyFile:            cfg[syslogTLSKey],
		InsecureSkipVerify: skipVerify,
	}

	return tlsconfig.Client(opts)
}

func parseSyslogLogFormat(logFormat, proto string) (syslog.Formatter, syslog.Framer, error) {
	switch logFormat {
	case "":
		return syslog.UnixFormatter, syslog.DefaultFramer, nil
	case syslogFormatRFC3164:
		return syslog.RFC3164Formatter, syslog.DefaultFramer, nil
	case syslogFormatRFC5424:
		if proto == syslogSecureProto {
			return syslog.RFC5424FormatterWithAppNameAsTag, syslog.RFC5425MessageLengthFramer, nil
		}
		return syslog.RFC5424FormatterWithAppNameAsTag, syslog.DefaultFramer, nil
	case syslogFormatRFC5424Micro:
		if proto == syslogSecureProto {
			return syslog.RFC5424MicroFormatterWithAppNameAsTag, syslog.RFC5425MessageLengthFramer, nil
		}
		return syslog.RFC5424MicroFormatterWithAppNameAsTag, syslog.DefaultFramer, nil
	default:
		return nil, nil, errors.New("invalid syslog format")
	}
}
