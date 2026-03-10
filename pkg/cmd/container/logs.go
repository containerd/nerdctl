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

package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"

	containerd "github.com/containerd/containerd/v2/client"
	"github.com/containerd/errdefs"
	"github.com/containerd/log"

	"github.com/containerd/nerdctl/v2/pkg/api/types"
	"github.com/containerd/nerdctl/v2/pkg/api/types/cri"
	"github.com/containerd/nerdctl/v2/pkg/clientutil"
	"github.com/containerd/nerdctl/v2/pkg/idutil/containerwalker"
	"github.com/containerd/nerdctl/v2/pkg/labels"
	"github.com/containerd/nerdctl/v2/pkg/labels/k8slabels"
	"github.com/containerd/nerdctl/v2/pkg/logging"
)

func Logs(ctx context.Context, client *containerd.Client, container string, options types.ContainerLogsOptions) error {
	dataStore, err := clientutil.DataStore(options.GOptions.DataRoot, options.GOptions.Address)
	if err != nil {
		return err
	}

	switch options.GOptions.Namespace {
	case "moby":
		log.G(ctx).Warn("Currently, `nerdctl logs` only supports containers created with `nerdctl run -d` or CRI")
	}

	stopChannel := make(chan os.Signal, 1)
	// catch OS signals:
	signal.Notify(stopChannel, syscall.SIGTERM, syscall.SIGINT)

	walker := &containerwalker.ContainerWalker{
		Client: client,
		OnFound: func(ctx context.Context, found containerwalker.Found) error {
			if found.MatchCount > 1 {
				return fmt.Errorf("multiple IDs found with provided prefix: %s", found.Req)
			}
			l, err := found.Container.Labels(ctx)
			if err != nil {
				return err
			}

			logPath, err := getLogPath(ctx, found.Container)
			if err != nil {
				return err
			}

			follow := options.Follow
			if follow {
				task, err := found.Container.Task(ctx, nil)
				if err != nil {
					if !errdefs.IsNotFound(err) {
						return err
					}
					follow = false
				} else {
					status, err := task.Status(ctx)
					if err != nil {
						return err
					}
					if status.Status != containerd.Running {
						follow = false
					} else {
						waitCh, err := task.Wait(ctx)
						if err != nil {
							return fmt.Errorf("failed to get wait channel for task %#v: %w", task, err)
						}

						// Setup goroutine to send stop event if container task finishes:
						go func() {
							<-waitCh
							// Wait for logger to process remaining logs after container exit
							if err = logging.WaitForLogger(dataStore, l[labels.Namespace], found.Container.ID()); err != nil {
								log.G(ctx).WithError(err).Error("failed to wait for logger shutdown")
							}
							log.G(ctx).Debugf("container task has finished, sending kill signal to log viewer")
							stopChannel <- os.Interrupt
						}()
					}
				}
			}

			var detailPrefix string
			if options.Details {
				if logConfigJSON, ok := l["nerdctl/log-config"]; ok {
					type logConfig struct {
						Opts map[string]string `json:"opts"`
					}

					e, err := getContainerEnvs(ctx, found.Container)
					if err != nil {
						return err
					}

					var logCfg logConfig
					var optPairs []string

					if err := json.Unmarshal([]byte(logConfigJSON), &logCfg); err == nil {
						envOpts, labelOpts := getLogOpts(logCfg.Opts)

						for _, v := range envOpts {
							if env, ok := e[v]; ok {
								optPairs = append(optPairs, fmt.Sprintf("%s=%s", v, env))
							}
						}

						for _, v := range labelOpts {
							if label, ok := l[v]; ok {
								optPairs = append(optPairs, fmt.Sprintf("%s=%s", v, label))
							}
						}

						if len(optPairs) > 0 {
							sort.Strings(optPairs)
							detailPrefix = strings.Join(optPairs, ",")
						}
					} else {
						log.L.Warn("failed to parse `--details` option, detailed information might not be displayed")
					}
				}
			}

			logViewOpts := logging.LogViewOptions{
				ContainerID:       found.Container.ID(),
				Namespace:         l[labels.Namespace],
				DatastoreRootPath: dataStore,
				LogPath:           logPath,
				Follow:            follow,
				Timestamps:        options.Timestamps,
				Tail:              options.Tail,
				Since:             options.Since,
				Until:             options.Until,
				Details:           options.Details,
				DetailPrefix:      &detailPrefix,
			}
			logViewer, err := logging.InitContainerLogViewer(l, logViewOpts, stopChannel, options.GOptions.Experimental)
			if err != nil {
				return err
			}

			return logViewer.PrintLogsTo(options.Stdout, options.Stderr)
		},
	}
	n, err := walker.Walk(ctx, container)
	if err != nil {
		return err
	} else if n == 0 {
		return fmt.Errorf("no such container %s", container)
	}
	return nil
}

func getLogPath(ctx context.Context, container containerd.Container) (string, error) {
	extensions, err := container.Extensions(ctx)
	if err != nil {
		return "", fmt.Errorf("get extensions for container %s,failed: %#v", container.ID(), err)
	}
	metaData := extensions[k8slabels.ContainerMetadataExtension]
	var meta cri.ContainerMetadata
	if metaData != nil {
		err = meta.UnmarshalJSON(metaData.GetValue())
		if err != nil {
			return "", fmt.Errorf("unmarshal extensions for container %s,failed: %#v", container.ID(), err)
		}
	}

	return meta.LogPath, nil
}

func getContainerEnvs(ctx context.Context, container containerd.Container) (map[string]string, error) {
	envMap := make(map[string]string)

	spec, err := container.Spec(ctx)
	if err != nil {
		return nil, err
	}

	if spec.Process == nil {
		return envMap, nil
	}

	for _, env := range spec.Process.Env {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	return envMap, nil
}

func getLogOpts(logOpts map[string]string) ([]string, []string) {
	var envOpts []string
	var labelOpts []string

	for k, v := range logOpts {
		lowerKey := strings.ToLower(k)
		if lowerKey == "env" {
			envNames := strings.Split(v, ",")
			envOpts = append(envOpts, envNames...)
		}

		if lowerKey == "labels" {
			labelNames := strings.Split(v, ",")
			labelOpts = append(labelOpts, labelNames...)
		}
	}

	return envOpts, labelOpts
}
