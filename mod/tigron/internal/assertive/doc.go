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

// Package assertive is an experimental, zero-dependencies assert library.
// Right now, it is not public and meant to be used only inside tigron.
// Consumers of tigron are free to use whatever assert library they want.
// In the future, this may become public for peeps who want `assert` to be
// bundled in.
package assertive
