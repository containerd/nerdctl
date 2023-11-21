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

package system

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/containerd/containerd"
	_ "github.com/containerd/containerd/api/events" // Register grpc event types
	"github.com/containerd/containerd/events"
	"github.com/containerd/log"
	"github.com/containerd/nerdctl/pkg/api/types"
	"github.com/containerd/nerdctl/pkg/formatter"
	"github.com/containerd/typeurl/v2"
)

// EventOut contains information about an event.
type EventOut struct {
	Timestamp time.Time
	Namespace string
	Topic     string
	Event     string
}

// Events is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/events/events.go
func Events(ctx context.Context, client *containerd.Client, options types.SystemEventsOptions) error {
	eventsClient := client.EventService()
	eventsCh, errCh := eventsClient.Subscribe(ctx)
	var tmpl *template.Template
	switch options.Format {
	case "":
		tmpl = nil
	case "raw", "table", "wide":
		return errors.New("unsupported format: \"raw\", \"table\", and \"wide\"")
	default:
		var err error
		tmpl, err = formatter.ParseTemplate(options.Format)
		if err != nil {
			return err
		}
	}
	for {
		var e *events.Envelope
		select {
		case e = <-eventsCh:
		case err := <-errCh:
			return err
		}
		if e != nil {
			var out []byte
			if e.Event != nil {
				v, err := typeurl.UnmarshalAny(e.Event)
				if err != nil {
					log.G(ctx).WithError(err).Warn("cannot unmarshal an event from Any")
					continue
				}
				out, err = json.Marshal(v)
				if err != nil {
					log.G(ctx).WithError(err).Warn("cannot marshal Any into JSON")
					continue
				}
			}
			if tmpl != nil {
				out := EventOut{e.Timestamp, e.Namespace, e.Topic, string(out)}
				var b bytes.Buffer
				if err := tmpl.Execute(&b, out); err != nil {
					return err
				}
				if _, err := fmt.Fprintln(options.Stdout, b.String()+"\n"); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintln(
					options.Stdout,
					e.Timestamp,
					e.Namespace,
					e.Topic,
					string(out),
				); err != nil {
					return err
				}
			}
		}
	}
}
