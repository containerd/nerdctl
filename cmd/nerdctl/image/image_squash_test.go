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

package image

import (
	"fmt"
	"testing"

	"gotest.tools/v3/assert"

	"github.com/containerd/nerdctl/mod/tigron/require"
	"github.com/containerd/nerdctl/mod/tigron/test"

	"github.com/containerd/nerdctl/v2/pkg/testutil"
	"github.com/containerd/nerdctl/v2/pkg/testutil/nerdtest"
)

func squashIdentifierName(identifier string) string {
	return fmt.Sprintf("%s-squash", identifier)
}

func secondCommitedIdentifierName(identifier string) string {
	return fmt.Sprintf("%s-second", identifier)
}

func TestSquash(t *testing.T) {
	testCase := nerdtest.Setup()

	testCase.SubTests = []*test.Case{
		{
			Description: "by last-n-layer",
			Require: require.All(
				require.Not(nerdtest.Docker),
				nerdtest.CGroup,
			),
			NoParallel: true,
			Cleanup: func(data test.Data, helpers test.Helpers) {
				identifier := data.Identifier()
				secondIdentifier := secondCommitedIdentifierName(identifier)
				squashIdentifier := squashIdentifierName(identifier)
				helpers.Anyhow("rm", "-f", identifier)
				helpers.Anyhow("rm", "-f", secondIdentifier)
				helpers.Anyhow("rm", "-f", squashIdentifier)

				helpers.Anyhow("rmi", "-f", secondIdentifier)
				helpers.Anyhow("rmi", "-f", identifier)
				helpers.Anyhow("rmi", "-f", squashIdentifier)
				helpers.Anyhow("image", "prune", "-f")
			},
			Setup: func(data test.Data, helpers test.Helpers) {
				identifier := data.Identifier()
				helpers.Ensure("run", "-d", "--name", identifier, testutil.CommonImage, "sleep", nerdtest.Infinity)
				helpers.Ensure("exec", identifier, "sh", "-euxc", `echo hello-first-commit > /foo`)
				helpers.Ensure("commit", "-c", `CMD ["cat", "/foo"]`, "-m", `first commit`, "--pause=true", identifier, identifier)
				out := helpers.Capture("run", "--rm", identifier)
				assert.Equal(t, out, "hello-first-commit\n")

				secondIdentifier := secondCommitedIdentifierName(identifier)
				helpers.Ensure("run", "-d", "--name", secondIdentifier, identifier, "sleep", nerdtest.Infinity)
				helpers.Ensure("exec", secondIdentifier, "sh", "-euxc", `echo hello-second-commit > /bar && echo hello-squash-commit > /foo`)
				helpers.Ensure("commit", "-c", `CMD ["cat", "/foo", "/bar"]`, "-m", `second commit`, "--pause=true", secondIdentifier, secondIdentifier)
				out = helpers.Capture("run", "--rm", secondIdentifier)
				assert.Equal(t, out, "hello-squash-commit\nhello-second-commit\n")

				squashIdentifier := squashIdentifierName(identifier)
				helpers.Ensure("image", "squash", "-n", "2", "-m", "squash commit", secondIdentifier, squashIdentifier)
				out = helpers.Capture("run", "--rm", squashIdentifier)
				assert.Equal(t, out, "hello-squash-commit\nhello-second-commit\n")
			},
			Command: func(data test.Data, helpers test.Helpers) test.TestableCommand {
				identifier := data.Identifier()

				squashIdentifier := squashIdentifierName(identifier)
				return helpers.Command("image", "history", "--human=true", "--format=json", squashIdentifier)
			},
			Expected: test.Expects(0, nil, func(stdout string, info string, t *testing.T) {
				history, err := decode(stdout)
				assert.NilError(t, err, info)
				assert.Equal(t, len(history), 3, info)
				assert.Equal(t, history[0].Comment, "squash commit", info)
			}),
		},
	}

	testCase.Run(t)
}
