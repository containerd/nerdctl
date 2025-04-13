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

// Package highk (for "high-κ dielectric") is a highly experimental leak detection library (for file descriptors and go
// routines).
// It is purely internal for now and used only as part of the tests for tigron.
// TODO:
// - get rid of lsof and implement in go
// - investigate feasibility of adding automatic leak detection for any test using tigron
// - investigate feasibility of adding leak detection for tested binaries
// - review usefulness of uber goroutines leak library
package highk
