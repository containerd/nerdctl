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

package event

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"text/template"
	"time"

	"github.com/containerd/containerd/events"
	"github.com/containerd/containerd/log"
	nerdClient "github.com/containerd/nerdctl/cmd/nerdctl/client"
	"github.com/containerd/nerdctl/cmd/nerdctl/utils/fmtutil"
	"github.com/containerd/typeurl"

	"github.com/spf13/cobra"

	// Register grpc event types
	_ "github.com/containerd/containerd/api/events"
)

func NewEventsCommand() *cobra.Command {
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
	eventsCommand.RegisterFlagCompletionFunc("format", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"json"}, cobra.ShellCompDirectiveNoFileComp
	})
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
	client, ctx, cancel, err := nerdClient.NewClient(cmd)
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
	case "raw", "table", "wide":
		return errors.New("unsupported format: \"raw\", \"table\", and \"wide\"")
	default:
		tmpl, err = fmtutil.ParseTemplate(format)
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
