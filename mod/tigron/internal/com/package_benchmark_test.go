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

package com_test

import (
	"context"
	"testing"

	"github.com/containerd/nerdctl/mod/tigron/internal/com"
)

// FIXME: this requires go 1.24 - uncomment when go 1.23 is out of support
// func BenchmarkCommand(b *testing.B) {
//	for b.Loop() {
//		cmd := com.Command{
//			Binary: "true",
//		}
//
//		_ = cmd.Run()
//		_, _ = cmd.Wait()
//	}
// }

func BenchmarkCommandParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			cmd := &com.Command{
				Binary: "true",
			}
			_ = cmd.Run(context.Background())
			_, _ = cmd.Wait()
		}
	})
}
