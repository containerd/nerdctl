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

package logging

import (
	"github.com/containerd/containerd/v2/core/runtime/v2/logging"
)

type NoneLogger struct {
	Opts map[string]string
}

func (n *NoneLogger) Init(dataStore, ns, id string) error {
	return nil
}

func (n *NoneLogger) PreProcess(dataStore string, config *logging.Config) error {
	return nil
}

func (n *NoneLogger) Process(stdout <-chan string, stderr <-chan string) error {
	return nil
}

func (n *NoneLogger) PostProcess() error {
	return nil
}

func NoneLogOptsValidate(_ map[string]string) error {
	return nil
}
