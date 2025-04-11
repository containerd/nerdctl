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

// Package mimicry provides a very rough and rudimentary mimicry library to help with internal tigron testing.
// It does not require generation, does not abuse reflect (too much), and keeps the amount of boilerplate baloney to a
// minimum.
// This is NOT a generic mock library. Use something else if you need one.
package mimicry
