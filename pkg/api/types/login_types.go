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

package types

type LoginCommandOptions struct {
	// GOptions is the global options.
	GOptions GlobalCommandOptions
	// ServerAddress is the server address to log in to.
	ServerAddress string
	// Username is the username to log in as.
	//
	// If it's empty, it will be inferred from the default auth config.
	// If nothing is in the auth config, the user will be prompted to provide it.
	Username string
	// Password is the password of the user.
	//
	// If it's empty, the user will be prompted to provide it.
	Password string
}
