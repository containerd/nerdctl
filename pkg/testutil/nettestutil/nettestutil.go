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

package nettestutil

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/containerd/containerd/errdefs"
)

func HTTPGet(urlStr string, attempts int, insecure bool) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	if attempts < 1 {
		return nil, errdefs.ErrInvalidArgument
	}
	client := &http.Client{
		Timeout: 3 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: insecure,
			},
		},
	}
	for i := 0; i < attempts; i++ {
		resp, err = client.Get(urlStr)
		if err == nil {
			return resp, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("error after %d attempts: %w", attempts, err)
}
