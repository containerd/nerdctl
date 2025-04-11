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

package tig

// T is what Tigron needs from a testing implementation (*testing.T obviously satisfies it).
//
// Expert note: using the testing.TB interface instead is tempting, but not possible, as the go authors made it
// impossible to implement (by declaring a private method on it):
// https://cs.opensource.google/go/go/+/refs/tags/go1.24.2:src/testing/testing.go;l=913-939
// Generally speaking, interfaces in go should be defined by the consumer, not the producer.
// Depending on producers' interfaces make them much harder to change for the producer, harder (or impossible) to mock,
// and decreases modularity.
// On the other hand, consumer defined interfaces allows to remove direct dependencies on implementation and encourages
// depending on abstraction instead, and reduces the interface size of what has to be mocked to just what is actually
// needed.
// This is a fundamental difference compared to traditional compiled languages that forces code to declare which
// interfaces it implements, while go interfaces are more a form of duck-typing.
// See https://www.airs.com/blog/archives/277 for more.
type T interface {
	Helper()
	FailNow()
	Fail()
	Log(args ...any)
	Name() string
	TempDir() string
}
