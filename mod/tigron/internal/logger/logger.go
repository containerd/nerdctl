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

package logger

import (
	"time"
)

// Logger describes a passed logger, useful only for debugging.
type Logger interface {
	Log(args ...any)
	Helper()
}

// ConcreteLogger is a simple struct allowing to set additional metadata for a Logger.
type ConcreteLogger struct {
	meta       []any
	wrappedLog Logger
}

// Set allows attaching metadata to the logger display.
func (cl *ConcreteLogger) Set(key, value string) *ConcreteLogger {
	return &ConcreteLogger{
		meta:       append(cl.meta, "["+key+"="+value+"]"),
		wrappedLog: cl.wrappedLog,
	}
}

// Log prints a message using the Log method of the embedded Logger.
func (cl *ConcreteLogger) Log(args ...any) {
	if cl.wrappedLog != nil {
		cl.wrappedLog.Helper()
		cl.wrappedLog.Log(
			append(
				append([]any{"[" + time.Now().Format(time.RFC3339) + "]"}, cl.meta...),
				args...)...)
	}
}

// Helper is called so that traces from t.Log are not linking to the logger methods themselves.
func (cl *ConcreteLogger) Helper() {
	if cl.wrappedLog != nil {
		cl.wrappedLog.Helper()
	}
}

// NewLogger returns a new concrete logger from a struct satisfying the Logger interface.
func NewLogger(logger Logger) *ConcreteLogger {
	return &ConcreteLogger{
		wrappedLog: logger,
	}
}
