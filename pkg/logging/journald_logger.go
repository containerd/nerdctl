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
	"io"
	"os/exec"
	"strconv"
	"sync"
	"text/template"
	"time"

	"github.com/containerd/containerd/runtime/v2/logging"
	"github.com/coreos/go-systemd/v22/journal"
	"github.com/docker/cli/templates"
)

type JournaldLogger struct {
	Opts map[string]string
}

type identifier struct {
	ID        string
	FullID    string
	Namespace string
}

func (journaldLogger *JournaldLogger) Process(dataStore string, config *logging.Config) error {
	if !journal.Enabled() {
		return errors.New("the local systemd journal is not available for logging.")
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

func FetchLogs(journalctlArgs []string, wStdoutPipe, wStderrPipe io.WriteCloser, logsEOFChan chan<- struct{}) error {
	journalctl, err := exec.LookPath("journalctl")
	if err != nil {
		return err
	}

	cmd := exec.Command(journalctl, journalctlArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	go io.Copy(wStdoutPipe, stdout)
	go io.Copy(wStderrPipe, stderr)

	if err := cmd.Start(); err != nil {
		return err
	}

	go func() error {
		if err := cmd.Wait(); err != nil {
			return err
		}
		logsEOFChan <- struct{}{}
		return nil
	}()

	return nil
}

func PrepareJournalCtlDate(t string) (string, error) {
	i, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return "", err
	}
	tm := time.Unix(i, 0)
	s := tm.Format("2006-01-02 15:04:05")
	return s, nil
}
