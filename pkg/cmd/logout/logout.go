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

package logout

import (
	"context"

	"github.com/containerd/nerdctl/v2/pkg/dockerutil"
)

func Logout(ctx context.Context, logoutServer string) (map[string]error, error) {
	reg, err := dockerutil.Parse(logoutServer)
	if err != nil {
		return nil, err
	}

	credentialsStore, err := dockerutil.New("")
	if err != nil {
		return nil, err
	}

	return credentialsStore.Erase(reg)
}

func ShellCompletion() ([]string, error) {
	credentialsStore, err := dockerutil.New("")
	if err != nil {
		return nil, err
	}

	return credentialsStore.ShellCompletion()
}
