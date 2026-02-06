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
	"encoding/csv"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// GPUReq is a request for GPUs.
type GPUReq struct {
	Count        int
	DeviceIDs    []string
	Capabilities []string
}

func (req *GPUReq) toCDIDeviceIDs(vendor string) []string {
	var cdiDeviceIDs []string
	for _, id := range req.normalizeDeviceIDs() {
		cdiDeviceIDs = append(cdiDeviceIDs, vendor+"/gpu="+id)
	}
	return cdiDeviceIDs
}

func (req *GPUReq) normalizeDeviceIDs() []string {
	if len(req.DeviceIDs) > 0 {
		return req.DeviceIDs
	}
	if req.Count < 0 {
		return []string{"all"}
	}
	var ids []string
	for i := 0; i < req.Count; i++ {
		ids = append(ids, fmt.Sprintf("%d", i))
	}

	return ids
}

// ParseGPUOptCSV parses a GPU option from CSV.
func ParseGPUOptCSV(value string) (*GPUReq, error) {
	csvReader := csv.NewReader(strings.NewReader(value))
	fields, err := csvReader.Read()
	if err != nil {
		return nil, err
	}

	var (
		req  GPUReq
		seen = map[string]struct{}{}
	)
	for _, field := range fields {
		parts := strings.SplitN(field, "=", 2)
		key := parts[0]
		if _, ok := seen[key]; ok {
			return nil, fmt.Errorf("gpu request key '%s' can be specified only once", key)
		}
		seen[key] = struct{}{}

		if len(parts) == 1 {
			seen["count"] = struct{}{}
			req.Count, err = parseCount(key)
			if err != nil {
				return nil, err
			}
			continue
		}

		value := parts[1]
		switch key {
		case "driver":
			if value != "nvidia" {
				return nil, fmt.Errorf("invalid driver %q: \"nvidia\" is only supported", value)
			}
		case "count":
			req.Count, err = parseCount(value)
			if err != nil {
				return nil, err
			}
		case "device":
			req.DeviceIDs = strings.Split(value, ",")
		case "capabilities":
			req.Capabilities = strings.Split(value, ",")
		case "options":
			// This option is allowed but not used for gpus.
			// Please see also: https://github.com/moby/moby/pull/38828
		default:
			return nil, fmt.Errorf("unexpected key '%s' in '%s'", key, field)
		}
	}

	if req.Count != 0 && len(req.DeviceIDs) > 0 {
		return nil, errors.New("cannot set both Count and DeviceIDs on device request")
	}
	if _, ok := seen["count"]; !ok && len(req.DeviceIDs) == 0 {
		req.Count = 1
	}

	return &req, nil
}

func parseGPUOpts(gpuOpts []string) ([]string, error) {
	if len(gpuOpts) == 0 {
		return nil, nil
	}

	vendor := detectGPUVendorFromCDI()
	if vendor == "" {
		return nil, fmt.Errorf("no known GPU vendor found in CDI specs")
	}

	gpuCDIDevices := []string{}
	for _, gpu := range gpuOpts {
		req, err := ParseGPUOptCSV(gpu)
		if err != nil {
			return nil, err
		}
		gpuCDIDevices = append(gpuCDIDevices, req.toCDIDeviceIDs(vendor)...)
	}
	return gpuCDIDevices, nil
}

func parseCount(s string) (int, error) {
	if s == "all" {
		return -1, nil
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return i, fmt.Errorf("count must be an integer: %w", err)
	}
	return i, nil
}
