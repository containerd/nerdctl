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
	"fmt"
	"os"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/containerd/nerdctl/mod/tigron/internal/highk"
)

func TestMain(m *testing.M) {
	// Prep exit code
	exitCode := 0
	defer func() { os.Exit(exitCode) }()

	// Configure logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).
		With().
		Str("library", "tigron").
		Logger()

	zerolog.SetGlobalLevel(zerolog.FatalLevel)

	switch os.Getenv("LOG_LEVEL") {
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}

	var (
		snapFile      *os.File
		before, after []byte
	)

	if os.Getenv("EXPERIMENTAL_HIGHK_FD") != "" {
		snapFile, _ = os.CreateTemp("", "fileleaks")
		before, _ = highk.SnapshotOpenFiles(snapFile)
	}

	exitCode = m.Run()

	if exitCode != 0 {
		return
	}

	if os.Getenv("EXPERIMENTAL_HIGHK_FD") != "" {
		after, _ = highk.SnapshotOpenFiles(snapFile)
		diff := highk.Diff(string(before), string(after))

		if len(diff) != 0 {
			zerolog.SetGlobalLevel(zerolog.ErrorLevel)

			log.Error().Msg("Leaking file descriptors")

			for _, file := range diff {
				log.Error().Str("file", file).Msg("leaked")
			}

			exitCode = 1
		}
	}

	if err := highk.FindGoRoutines(); err != nil {
		log.Error().Msg("Leaking go routines")

		_, _ = fmt.Fprint(os.Stderr, err.Error())

		exitCode = 1
	}
}
