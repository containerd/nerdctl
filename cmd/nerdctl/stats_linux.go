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
	"errors"

	v1 "github.com/containerd/cgroups/stats/v1"
	v2 "github.com/containerd/cgroups/v2/stats"
	"github.com/containerd/nerdctl/pkg/statsutil"
)

var (
	data       *v1.Metrics
	data2      *v2.Metrics
	statsEntry statsutil.StatsEntry
)

func renderStatsEntry(previousStats map[string]uint64, anydata interface{}) (statsutil.StatsEntry, error) {

	switch v := anydata.(type) {
	case *v1.Metrics:
		data = v
	case *v2.Metrics:
		data2 = v
	default:
		return statsutil.StatsEntry{}, errors.New("cannot convert metric data to cgroups.Metrics")
	}
	var err error
	if data != nil {
		statsEntry, err = statsutil.SetCgroupStatsFields(previousStats["CgroupCPU"], previousStats["CgroupSystem"], data)
		previousStats["CgroupCPU"] = data.CPU.Usage.Total
		previousStats["CgroupSystem"] = data.CPU.Usage.Kernel
		if err != nil {
			return statsutil.StatsEntry{}, err
		}
	} else if data2 != nil {
		statsEntry, err = statsutil.SetCgroup2StatsFields(previousStats["Cgroup2CPU"], previousStats["Cgroup2System"], data2)
		previousStats["Cgroup2CPU"] = data2.CPU.UsageUsec * 1000
		previousStats["Cgroup2System"] = data2.CPU.SystemUsec * 1000
		if err != nil {
			return statsutil.StatsEntry{}, err
		}
	}

	return statsEntry, nil
}
