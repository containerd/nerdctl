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

package jsonfile

import (
	"container/ring"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/containerd/log"
	timetypes "github.com/docker/docker/api/types/time"
)

// Entry is compatible with Docker "json-file" logs
type Entry struct {
	Log    string    `json:"log,omitempty"`    // line, including "\r\n"
	Stream string    `json:"stream,omitempty"` // "stdout" or "stderr"
	Time   time.Time `json:"time"`             // e.g. "2020-12-11T20:29:41.939902251Z"
}

func Path(dataStore, ns, id string) string {
	// the file name corresponds to Docker
	return filepath.Join(dataStore, "containers", ns, id, id+"-json.log")
}

func Encode(stdout <-chan string, stderr <-chan string, writer io.Writer) error {
	enc := json.NewEncoder(writer)
	var encMu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(2)
	f := func(dataChan <-chan string, name string) {
		defer wg.Done()
		e := &Entry{
			Stream: name,
		}
		for logEntry := range dataChan {
			e.Log = logEntry + "\n"
			e.Time = time.Now().UTC()
			encMu.Lock()
			encErr := enc.Encode(e)
			encMu.Unlock()
			if encErr != nil {
				log.L.WithError(encErr).Errorf("failed to encode JSON")
				return
			}
		}
	}
	go f(stdout, "stdout")
	go f(stderr, "stderr")
	wg.Wait()
	return nil
}

func writeEntry(e *Entry, stdout, stderr io.Writer, refTime time.Time, timestamps bool, since string, until string) error {
	output := []byte{}

	if since != "" {
		ts, err := timetypes.GetTimestamp(since, refTime)
		if err != nil {
			return fmt.Errorf("invalid value for \"since\": %w", err)
		}
		v := strings.Split(ts, ".")
		i, err := strconv.ParseInt(v[0], 10, 64)
		if err != nil {
			return err
		}
		if e.Time.Before(time.Unix(i, 0)) {
			return nil
		}
	}

	if until != "" {
		ts, err := timetypes.GetTimestamp(until, refTime)
		if err != nil {
			return fmt.Errorf("invalid value for \"until\": %w", err)
		}
		v := strings.Split(ts, ".")
		i, err := strconv.ParseInt(v[0], 10, 64)
		if err != nil {
			return err
		}
		if e.Time.After(time.Unix(i, 0)) {
			return nil
		}
	}

	if timestamps {
		output = append(output, []byte(e.Time.Format(time.RFC3339Nano))...)
		output = append(output, ' ')
	}

	output = append(output, []byte(e.Log)...)

	var writeTo io.Writer
	switch e.Stream {
	case "stdout":
		writeTo = stdout
	case "stderr":
		writeTo = stderr
	default:
		log.L.Errorf("unknown stream name %q, entry=%+v", e.Stream, e)
	}

	if writeTo != nil {
		_, err := writeTo.Write(output)
		return err
	}
	return nil
}

func Decode(stdout, stderr io.Writer, r io.Reader, timestamps bool, since string, until string, tail uint) error {
	var buff *ring.Ring
	if tail != 0 {
		buff = ring.New(int(tail))
	}

	dec := json.NewDecoder(r)
	now := time.Now()
	for {
		var e Entry
		if err := dec.Decode(&e); err == io.EOF {
			break
		} else if err != nil {
			return err
		}

		if buff == nil {
			// Write out the entry directly
			err := writeEntry(&e, stdout, stderr, now, timestamps, since, until)
			if err != nil {
				log.L.Errorf("error while writing log entry to output stream: %s", err)
			}
		} else {
			// Else place the entry in a ring buffer
			buff.Value = &e
			buff = buff.Next()
		}
	}

	if buff != nil {
		// The ring should now contain up to `tail` elements and be set to
		// internally point to the oldest element in the ring.
		buff.Do(func(e interface{}) {
			if e == nil {
				// unallocated ring element
				return
			}
			cast, ok := e.(*Entry)
			if !ok {
				log.L.Errorf("failed to cast Entry struct: %#v", e)
				return
			}

			err := writeEntry(cast, stdout, stderr, now, timestamps, since, until)
			if err != nil {
				log.L.Errorf("error while writing log entry to output stream: %s", err)
			}
		})
	}
	return nil
}
