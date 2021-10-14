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
	"bytes"
	"encoding/json"
	"fmt"
	"text/template"
	"time"

	"github.com/containerd/containerd/events"
	"github.com/containerd/containerd/log"
	"github.com/containerd/typeurl"
	"github.com/docker/cli/templates"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	// Register grpc event types
	_ "github.com/containerd/containerd/api/events"
)

func newEventsCommand() *cobra.Command {
	shortHelp := `Get real time events from the server`
	longHelp := shortHelp + "\nNOTE: The output format is not compatible with Docker."
	var eventsCommand = &cobra.Command{
		Use:           "events",
		Args:          cobra.NoArgs,
		Short:         shortHelp,
		Long:          longHelp,
		RunE:          eventsAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	eventsCommand.Flags().String("format", "", "Format the output using the given Go template, e.g, '{{json .}}'")
	return eventsCommand
}

type Out struct {
	Timestamp time.Time
	Namespace string
	Topic     string
	Event     string
}

// eventsActions is from https://github.com/containerd/containerd/blob/v1.4.3/cmd/ctr/commands/events/events.go
func eventsAction(cmd *cobra.Command, args []string) error {
	if len(args) != 0 {
		return errors.Errorf("accepts no arguments")
	}

	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	eventsClient := client.EventService()
	eventsCh, errCh := eventsClient.Subscribe(ctx)

	var tmpl *template.Template
	format, err := cmd.Flags().GetString("format")
	if err != nil {
		return err
	}
	switch format {
	case "":
		tmpl = nil
	case "raw", "table":
		return errors.New("unsupported format: \"raw\" and \"table\"")
	default:
		tmpl, err = templates.Parse(format)
		if err != nil {
			return err
		}
	}
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
			if tmpl != nil {
				out := Out{e.Timestamp, e.Namespace, e.Topic, string(out)}
				var b bytes.Buffer
				if err := tmpl.Execute(&b, out); err != nil {
					return err
				}
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), b.String()+"\n"); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintln(
					cmd.OutOrStdout(),
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
