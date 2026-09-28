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
	"strconv"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/moby/buildkit/frontend/dockerfile/shell"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// applyChanges applies each --change to cfg in order, using BuildKit's
// Dockerfile parser so the syntax matches what build accepts.
func applyChanges(cfg *ocispec.ImageConfig, changes []string) error {
	for _, c := range changes {
		if err := applyChange(cfg, c); err != nil {
			return fmt.Errorf("invalid --change %q: %w", c, err)
		}
	}
	return nil
}

// applyChange parses a single change and writes it into cfg.
func applyChange(cfg *ocispec.ImageConfig, change string) error {
	cmd, err := parseInstruction(change)
	if err != nil || cmd == nil {
		return err
	}
	if err := expand(cfg, cmd); err != nil {
		return err
	}
	return dispatch(cfg, cmd)
}

// parseInstruction parses one change into a typed instruction. More than one
// is rejected, so a change cannot smuggle in a second.
func parseInstruction(change string) (any, error) {
	res, err := parser.Parse(strings.NewReader(change))
	if err != nil {
		return nil, err
	}
	switch len(res.AST.Children) {
	case 0:
		return nil, nil
	case 1:
	default:
		return nil, fmt.Errorf("must be a single instruction, got %d", len(res.AST.Children))
	}
	return instructions.ParseInstruction(res.AST.Children[0])
}

// expand unquotes the instruction's words and resolves $VAR against the config
// built so far; the parser hands back raw tokens. An unset variable expands to
// empty, as it does in docker import.
func expand(cfg *ocispec.ImageConfig, cmd any) error {
	expandable, ok := cmd.(interface {
		Expand(instructions.SingleWordExpander) error
	})
	if !ok {
		return nil
	}
	lex := shell.NewLex('\\')
	env := shell.EnvsFromSlice(cfg.Env)
	return expandable.Expand(func(word string) (string, error) {
		w, _, err := lex.ProcessWord(word, env)
		return w, err
	})
}

// dispatch writes one parsed instruction into cfg. The type switch is the
// allow-list: anything that builds rather than describes has no case.
func dispatch(cfg *ocispec.ImageConfig, cmd any) error {
	switch c := cmd.(type) {
	case *instructions.CmdCommand:
		cfg.Cmd = cmdLine(c.ShellDependantCmdLine)
	case *instructions.EntrypointCommand:
		cfg.Entrypoint = cmdLine(c.ShellDependantCmdLine)
	case *instructions.EnvCommand:
		for _, kv := range c.Env {
			cfg.Env = setEnv(cfg.Env, kv.Key, kv.Value)
		}
	case *instructions.LabelCommand:
		if cfg.Labels == nil && len(c.Labels) > 0 {
			cfg.Labels = map[string]string{}
		}
		for _, kv := range c.Labels {
			cfg.Labels[kv.Key] = kv.Value
		}
	case *instructions.ExposeCommand:
		// Ports stay raw tokens, so ranges and the default proto are ours.
		return parseExpose(cfg, c.Ports)
	case *instructions.VolumeCommand:
		if cfg.Volumes == nil && len(c.Volumes) > 0 {
			cfg.Volumes = map[string]struct{}{}
		}
		for _, v := range c.Volumes {
			cfg.Volumes[v] = struct{}{}
		}
	case *instructions.UserCommand:
		cfg.User = c.User
	case *instructions.WorkdirCommand:
		cfg.WorkingDir = c.Path
	case *instructions.StopSignalCommand:
		cfg.StopSignal = c.Signal
	default:
		return fmt.Errorf("the %s instruction is not supported by import", instructionName(cmd))
	}
	return nil
}

// cmdLine returns the argv for CMD/ENTRYPOINT, wrapping the shell form in
// "/bin/sh -c" like Docker.
func cmdLine(c instructions.ShellDependantCmdLine) []string {
	if len(c.CmdLine) == 0 {
		return nil
	}
	if c.PrependShell {
		return append([]string{"/bin/sh", "-c"}, c.CmdLine...)
	}
	return c.CmdLine
}

// instructionName returns the Dockerfile keyword for a parsed instruction, for
// use in error messages. FROM parses to a stage rather than a command, so it
// has no Name of its own.
func instructionName(cmd any) string {
	switch c := cmd.(type) {
	case instructions.Command:
		return strings.ToUpper(c.Name())
	case *instructions.Stage:
		return "FROM"
	default:
		return fmt.Sprintf("%T", cmd)
	}
}

// parseExpose adds each "port[/proto]" token to cfg.ExposedPorts, defaulting the
// protocol to tcp. A "start-end" port range is expanded to one entry per port,
// matching Docker's EXPOSE.
func parseExpose(cfg *ocispec.ImageConfig, ports []string) error {
	for _, tok := range ports {
		portSpec, proto := tok, "tcp"
		if p, pr, ok := strings.Cut(tok, "/"); ok {
			portSpec, proto = p, strings.ToLower(pr)
		}
		if proto != "tcp" && proto != "udp" && proto != "sctp" {
			return fmt.Errorf("invalid EXPOSE protocol %q", proto)
		}
		lo, hi, err := parsePortRange(portSpec)
		if err != nil {
			return err
		}
		if cfg.ExposedPorts == nil {
			cfg.ExposedPorts = map[string]struct{}{}
		}
		// uint32 counter so hi == 65535 does not wrap a uint16 into an endless loop.
		for p := lo; p <= hi; p++ {
			cfg.ExposedPorts[fmt.Sprintf("%d/%s", p, proto)] = struct{}{}
		}
	}
	return nil
}

// parsePortRange parses a single port or an inclusive "start-end" range into its
// low and high bounds.
func parsePortRange(s string) (uint32, uint32, error) {
	if loStr, hiStr, ok := strings.Cut(s, "-"); ok {
		lo, err1 := strconv.ParseUint(loStr, 10, 16)
		hi, err2 := strconv.ParseUint(hiStr, 10, 16)
		if err1 != nil || err2 != nil {
			return 0, 0, fmt.Errorf("invalid EXPOSE port range %q", s)
		}
		if lo > hi {
			return 0, 0, fmt.Errorf("invalid EXPOSE port range %q", s)
		}
		return uint32(lo), uint32(hi), nil
	}
	p, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid EXPOSE port %q", s)
	}
	return uint32(p), uint32(p), nil
}

// setEnv replaces the "key=" entry in env if present, otherwise appends it.
func setEnv(env []string, key, val string) []string {
	entry := key + "=" + val
	prefix := key + "="
	for i, e := range env {
		if strings.HasPrefix(e, prefix) {
			env[i] = entry
			return env
		}
	}
	return append(env, entry)
}
