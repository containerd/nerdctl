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
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/nerdctl/pkg/containerinspector"
	"github.com/containerd/nerdctl/pkg/namestore"
	"github.com/containerd/nerdctl/pkg/strutil"
	"github.com/spf13/cobra"
)

func newContainerPruneCommand() *cobra.Command {
	containerPruneCommand := &cobra.Command{
		Use:           "prune [flags]",
		Short:         "Removes all stopped containers",
		RunE:          containerPruneAction,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	containerPruneCommand.Flags().BoolP("force", "f", false, "Do not prompt for confirmation")
	containerPruneCommand.Flags().StringArray("filter", nil, "Provide filter values (e.g. 'until=<timestamp>')")
	return containerPruneCommand
}

func containerPruneAction(cmd *cobra.Command, _ []string) error {
	client, ctx, cancel, err := newClient(cmd)
	if err != nil {
		return err
	}
	defer cancel()
	dataStore, err := getDataStore(cmd)
	if err != nil {
		return err
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	filterMaps, err := checkFilter(cmd)
	if err != nil {
		return err
	}

	if !force {
		var confirm string
		fmt.Fprintf(cmd.OutOrStdout(), "%s", "WARNING! This will remove all stopped containers.\nAre you sure you want to continue? [y/N] ")
		fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)

		if strings.ToLower(confirm) != "y" {
			return nil
		}
	}
	ns, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}

	var deletedContainers []string
	containers, err := client.Containers(ctx)
	if err != nil {
		return err
	}
	for _, container := range containers {
		if filterMaps != nil {
			ok, err := getFilteredContainers(container, filterMaps, ctx)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
		}
		cont, err := containerinspector.Inspect(ctx, container)
		if err != nil {
			return err
		}
		if cont.Process == nil || (cont.Process != nil && cont.Process.Status.Status != containerd.Running) {
			containerNameStore, err := namestore.New(dataStore, ns)
			if err != nil {
				return err
			}
			stateDir, err := getContainerStateDirPath(cmd, dataStore, container.ID())
			if err != nil {
				return err
			}
			err = removeContainer(cmd, ctx, container, ns, "", force, dataStore, stateDir, containerNameStore, true)
			if err != nil {
				return err
			}
			deletedContainers = append(deletedContainers, container.ID())
		}
	}
	if deletedContainers != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Deleted Containers: \n")
		for _, elem := range deletedContainers {
			fmt.Fprintln(cmd.OutOrStdout(), elem)
		}
	}

	return nil
}
func checkFilter(cmd *cobra.Command) (map[string]map[string]string, error) {
	filters, err := cmd.Flags().GetStringArray("filter")
	if err != nil {
		return nil, err
	}
	filters = strutil.DedupeStrSlice(filters)
	filtersMap, err := convToMap(filters)
	if err != nil {
		return nil, err
	}
	return filtersMap, nil
}

func getFilteredContainers(container containerd.Container, filterMaps map[string]map[string]string, ctx context.Context) (bool, error) {
	for label, filterKey := range filterMaps {
		if label == "until" {
			if ok, err := untilCheck(container, filterKey, ctx); ok {
				continue
			} else {
				if err != nil {
					return false, err
				}
				return false, nil
			}

		} else if strings.Contains(label, "label") {
			if ok, err := labelCheck(container, filterKey, ctx, label); ok {
				continue
			} else {
				if err != nil {
					return false, err
				}
				return false, nil
			}
		}
	}
	return true, nil
}

func convToMap(values []string) (map[string]map[string]string, error) {
	labelMap := make(map[string]map[string]string, len(values))
	var once bool
	for i, value := range values {
		kv := strings.Split(value, "=")
		if kv[0] == "label" || kv[0] == "label!" {
			if len(kv) == 3 {
				labels := kv[1] + "=" + kv[2]
				labelMap[kv[0]+strconv.Itoa(i)] = strutil.ConvertKVStringsToMap([]string{labels})
			} else {
				labelMap[kv[0]+strconv.Itoa(i)] = strutil.ConvertKVStringsToMap([]string{kv[1]})
			}
		} else if kv[0] == "until" {
			if !once {
				labelMap[kv[0]] = strutil.ConvertKVStringsToMap([]string{kv[1]})
				once = true
			} else {
				err := fmt.Errorf("more than one until filter specified")
				return nil, err
			}
		} else {
			err := fmt.Errorf("invalid filter %q", kv[0])
			return nil, err
		}
	}
	return labelMap, nil
}

func untilCheck(container containerd.Container, filterMap map[string]string, ctx context.Context) (bool, error) {
	for filterValue, x := range filterMap {
		log.Println(x)
		infoCont, _ := container.Info(ctx)
		dur, err := time.ParseDuration(filterValue)
		if math.Abs(time.Until(infoCont.CreatedAt).Seconds()) >= dur.Seconds() {
			return true, nil
		}
		if dur == 0 {
			checkT, err := time.Parse("2006-01-02T15:04:05Z02:00", filterValue)
			if err == nil {
				goto check
			}
			checkT, err = time.Parse("2006-01-02T15:04:05", filterValue)
			if err == nil {
				goto check
			}
			checkT, err = time.Parse("2006-01-02", filterValue)
			if err == nil {
				goto check
			}
			checkT, err = time.Parse("2006-01-02Z02:00", filterValue)
			if err == nil {
				goto check
			}
			checkT, err = time.Parse("2006-01-02T15:04:05.999999999Z02:00", filterValue)
			if err != nil {
				return false, err
			}
		check:
			if checkT.After(infoCont.CreatedAt) {
				return true, nil
			}
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	return false, nil
}
func labelCheck(container containerd.Container, filterMap map[string]string, ctx context.Context, filter string) (bool, error) {
	for key, filterValue := range filterMap {
		contLabels, err := container.Labels(ctx)
		if err != nil {
			return false, err
		}
		if strings.Contains(filter, "label!") {
			if fVal, ok := contLabels[key]; ok {
				if filterValue != "" && fVal != filterValue {
					return true, nil
				}
				return false, nil
			}
			return true, nil
		}
		if fVal, ok := contLabels[key]; ok {
			if filterValue != "" && fVal == filterValue {
				return true, nil
			} else if filterValue != "" && fVal != filterValue {
				return false, nil
			}
			return true, nil
		}
	}
	return false, nil
}
