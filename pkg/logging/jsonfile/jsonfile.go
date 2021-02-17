/*
   Copyright (C) nerdctl authors.
   Copyright (C) containerd authors.

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

package jsonfile

import (
	"bufio"
	"encoding/json"
	"io"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// Entry is compatible with Docker "json-file" logs
type Entry struct {
	Log    string    `json:"log,omitempty"`    // line, including "\r\n"
	Stream string    `json:"stream,omitempty"` // "stdout" or "stderr"
	Time   time.Time `json:"time"`             // e.g. "2020-12-11T20:29:41.939902251Z"
}

func Path(dataRoot, ns, id string) string {
	// the file name corresponds to Docker
	return filepath.Join(dataRoot, "containers", ns, id, id+"-json.log")
}

func Encode(w io.Writer, stdout, stderr io.Reader) error {
	enc := json.NewEncoder(w)
	var encMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)
	f := func(r io.Reader, name string) {
		defer wg.Done()
		br := bufio.NewReader(r)
		e := &Entry{
			Stream: name,
		}
		for {
			line, err := br.ReadString(byte('\n'))
			if err != nil {
				logrus.WithError(err).Errorf("faild to read line from %q", name)
				return
			}
			e.Log = line
			e.Time = time.Now().UTC()
			encMu.Lock()
			encErr := enc.Encode(e)
			encMu.Unlock()
			if encErr != nil {
				logrus.WithError(err).Errorf("faild to encode JSON")
				return
			}
		}
	}
	go f(stdout, "stdout")
	go f(stderr, "stderr")
	wg.Wait()
	return nil
}

func Decode(stdout, stderr io.Writer, r io.Reader) error {
	dec := json.NewDecoder(r)
	for {
		var e Entry
		if err := dec.Decode(&e); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		switch e.Stream {
		case "stdout":
			stdout.Write([]byte(e.Log))
		case "stderr":
			stderr.Write([]byte(e.Log))
		default:
			logrus.Errorf("unknown stream name %q, entry=%+v", e.Stream, e)
		}
	}
	return nil
}
