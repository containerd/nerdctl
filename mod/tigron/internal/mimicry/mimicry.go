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

package mimicry

import (
	"reflect"
	"runtime"
	"strings"
	"time"
)

const callStackMaxDepth = 5

var _ Mocked = &Core{}

// Mocked is the interface representing a fully-mocking struct (both for Designer and Consumer).
type Mocked interface {
	Consumer
	Designer
}

// Function is a generics for any mockable function.
type Function[IN any, OUT any] = func(IN) OUT

// Consumer is the mock interface exposed to mock users.
// It defines a handful of methods to register a custom handler, get and reset calls reports.
type Consumer interface {
	Register(fun, handler any)
	Report(fun any) []*Call
	Reset()
}

// Designer is the mock interface that mock creators can use to write function boilerplate.
type Designer interface {
	Retrieve(args ...any) any
}

// Core is a concrete implementation that any mock struct can embed to satisfy Mocked.
// FIXME: this is not safe to use concurrently.
type Core struct {
	mockedFunctions map[string]any
	callsList       map[string][]*Call
}

// Reset does reset the callStack records for all functions.
func (mi *Core) Reset() {
	mi.callsList = make(map[string][]*Call)
}

// Report returns all Calls made to the referenced function.
func (mi *Core) Report(fun any) []*Call {
	fid := getFunID(fun)

	if mi.callsList == nil {
		mi.callsList = make(map[string][]*Call)
	}

	ret, ok := mi.callsList[fid]
	if !ok {
		ret = []*Call{}
	}

	return ret
}

// Retrieve returns a registered custom handler for that function if there is one.
func (mi *Core) Retrieve(args ...any) any {
	// Get the frames.
	pc := make([]uintptr, callStackMaxDepth)
	//nolint:mnd // Whatever mnd...
	n := runtime.Callers(2, pc)
	callersFrames := runtime.CallersFrames(pc[:n])
	// This is the frame associate with the mock currently calling retrieve, so, extract the short
	// name of it.
	frame, _ := callersFrames.Next()
	nm := strings.Split(frame.Function, ".")
	fid := nm[len(nm)-1]

	// Initialize callsList if need be
	if mi.callsList == nil {
		mi.callsList = make(map[string][]*Call)
	}

	// Now, get the remaining frames until we hit the go library or the call stack depth limit.
	frames := []*Frame{}

	for range callStackMaxDepth {
		frame, _ = callersFrames.Next()
		if isStd(frame.Function) {
			break
		}

		frames = append(frames, &Frame{
			File:     frame.File,
			Function: frame.Function,
			Line:     frame.Line,
		})
	}

	// Stuff into the call list.
	mi.callsList[fid] = append(mi.callsList[fid], &Call{
		Time:   time.Now(),
		Args:   args,
		Frames: frames,
	})

	// See if we have a registered handler and return it if so.
	if ret, ok := mi.mockedFunctions[fid]; ok {
		return ret
	}

	return nil
}

// Register does declare an explicit handler for that function.
func (mi *Core) Register(fun, handler any) {
	if mi.mockedFunctions == nil {
		mi.mockedFunctions = make(map[string]any)
	}

	mi.mockedFunctions[getFunID(fun)] = handler
}

func getFunID(fun any) string {
	// The point of keeping only the func name is to avoid type mismatch dependent on what interface
	// is used by the consumer.
	origin := runtime.FuncForPC(reflect.ValueOf(fun).Pointer()).Name()
	seg := strings.Split(origin, ".")
	origin = seg[len(seg)-1]

	return origin
}
