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

package helpers

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func Confirm(cmd *cobra.Command, message string) (bool, error) {
	message += "\nAre you sure you want to continue? [y/N] "
	_, err := fmt.Fprint(cmd.OutOrStdout(), message)
	if err != nil {
		return false, err
	}

	var confirm string
	_, err = fmt.Fscanf(cmd.InOrStdin(), "%s", &confirm)
	if err != nil {
		return false, err
	}
	return strings.ToLower(confirm) == "y", err
}
