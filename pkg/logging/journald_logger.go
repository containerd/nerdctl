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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"text/template"
	"time"

	"github.com/containerd/containerd/runtime/v2/logging"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/coreos/go-systemd/v22/journal"
	"github.com/docker/cli/templates"
	timetypes "github.com/docker/docker/api/types/time"
	"github.com/sirupsen/logrus"
)

var JournalDriverLogOpts = []string{
	Tag,
}

func JournalLogOptsValidate(logOptMap map[string]string) error {
	for key := range logOptMap {
		if !strutil.InStringSlice(JournalDriverLogOpts, key) {
			logrus.Warnf("log-opt %s is ignored for journald log driver", key)
		}
	}
	return nil
}

type JournaldLogger struct {
	Opts map[string]string
}

type identifier struct {
	ID        string
	FullID    string
	Namespace string
}

func (journaldLogger *JournaldLogger) Init(dataStore, ns, id string) error {
	return nil
}

func (journaldLogger *JournaldLogger) Process(dataStore string, config *logging.Config) error {
	if !journal.Enabled() {
		return errors.New("the local systemd journal is not available for logging")
	}
	shortID := config.ID[:12]
	var syslogIdentifier string
	if _, ok := journaldLogger.Opts[Tag]; !ok {
		syslogIdentifier = shortID
	} else {
		var tmpl *template.Template
		var err error
		tmpl, err = templates.Parse(journaldLogger.Opts[Tag])
		if err != nil {
			return err
		}

		if tmpl != nil {
			idn := identifier{
				ID:        shortID,
				FullID:    config.ID,
				Namespace: config.Namespace,
			}
			var b bytes.Buffer
			if err := tmpl.Execute(&b, idn); err != nil {
				return err
			}
			syslogIdentifier = b.String()
		}
	}
	// construct log metadata for the container
	vars := map[string]string{
		"SYSLOG_IDENTIFIER": syslogIdentifier,
	}
	var wg sync.WaitGroup
	wg.Add(2)
	f := func(wg *sync.WaitGroup, r io.Reader, pri journal.Priority, vars map[string]string) {
		defer wg.Done()
		s := bufio.NewScanner(r)
		for s.Scan() {
			if s.Err() != nil {
				return
			}
			journal.Send(s.Text(), pri, vars)
		}
	}
	// forward both stdout and stderr to the journal
	go f(&wg, config.Stdout, journal.PriInfo, vars)
	go f(&wg, config.Stderr, journal.PriErr, vars)

	wg.Wait()
	return nil
}

// Exec's `journalctl` with the provided arguments and hooks it up
// to the given stdout/stderr streams.
func FetchLogs(stdout, stderr io.Writer, journalctlArgs []string, stopChannel chan os.Signal) error {
	journalctl, err := exec.LookPath("journalctl")
	if err != nil {
		return fmt.Errorf("could not find `journalctl` executable in PATH: %s", err)
	}

	cmd := exec.Command(journalctl, journalctlArgs...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start journalctl command with args %#v: %s", journalctlArgs, err)
	}

	// Setup killing goroutine:
	go func() {
		<-stopChannel
		logrus.Debugf("killing journalctl logs process with PID: %#v", cmd.Process.Pid)
		cmd.Process.Kill()
	}()

	return nil
}

// Formats command line arguments for `journalctl` with the provided log viewing options and
// exec's and redirects `journalctl`s outputs to the provided io.Writers.
func viewLogsJournald(lvopts LogViewOptions, stdout, stderr io.Writer, stopChannel chan os.Signal) error {
	if !checkExecutableAvailableInPath("journalctl") {
		return fmt.Errorf("`journalctl` executable could not be found in PATH, cannot use Journald to view logs")
	}
	shortID := lvopts.ContainerID[:12]
	var journalctlArgs = []string{fmt.Sprintf("SYSLOG_IDENTIFIER=%s", shortID), "--output=cat"}
	if lvopts.Follow {
		journalctlArgs = append(journalctlArgs, "-f")
	}
	if lvopts.Since != "" {
		// using GetTimestamp from moby to keep time format consistency
		ts, err := timetypes.GetTimestamp(lvopts.Since, time.Now())
		if err != nil {
			return fmt.Errorf("invalid value for \"since\": %w", err)
		}
		date, err := prepareJournalCtlDate(ts)
		if err != nil {
			return err
		}
		journalctlArgs = append(journalctlArgs, "--since", date)
	}
	if lvopts.Timestamps {
		logrus.Warnf("unsupported Timestamps option for journald driver")
	}
	if lvopts.Until != "" {
		// using GetTimestamp from moby to keep time format consistency
		ts, err := timetypes.GetTimestamp(lvopts.Until, time.Now())
		if err != nil {
			return fmt.Errorf("invalid value for \"until\": %w", err)
		}
		date, err := prepareJournalCtlDate(ts)
		if err != nil {
			return err
		}
		journalctlArgs = append(journalctlArgs, "--until", date)
	}
	return FetchLogs(stdout, stderr, journalctlArgs, stopChannel)
}

func prepareJournalCtlDate(t string) (string, error) {
	i, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return "", err
	}
	tm := time.Unix(i, 0)
	s := tm.Format("2006-01-02 15:04:05")
	return s, nil
}
