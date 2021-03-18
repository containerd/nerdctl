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

package main

import (
	"encoding/json"
	"fmt"

	"github.com/containerd/containerd/events"
	"github.com/containerd/containerd/log"
	"github.com/containerd/typeurl"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"

	// Register grpc event types
	_ "github.com/containerd/containerd/api/events"
)

var eventsCommand = &cli.Command{
	Name:        "events",
	Usage:       "Get real time events from the server",
	Description: "NOTE: The output format is not compatible with Docker.",
	Action:      eventsAction,
}

// eventsActions is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/events/events.go
func eventsAction(clicontext *cli.Context) error {
	if clicontext.NArg() != 0 {
		return errors.Errorf("accepts no arguments")
	}

	client, ctx, cancel, err := newClient(clicontext)
	if err != nil {
		return err
	}
	defer cancel()
	eventsClient := client.EventService()
	eventsCh, errCh := eventsClient.Subscribe(ctx)
	for {
		var e *events.Envelope
		select {
		case e = <-eventsCh:
		case err = <-errCh:
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
			if _, err := fmt.Fprintln(
				clicontext.App.Writer,
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
