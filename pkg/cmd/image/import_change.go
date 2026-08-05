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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// applyChanges applies each Dockerfile-style change (as passed to --change) to
// cfg in order. It supports only the instructions representable in an OCI image
// config; Docker-only instructions and unknown ones are rejected.
func applyChanges(cfg *ocispec.ImageConfig, changes []string) error {
	for _, c := range changes {
		if err := applyChange(cfg, c); err != nil {
			return fmt.Errorf("invalid --change %q: %w", c, err)
		}
	}
	return nil
}

// applyChange parses a single "INSTRUCTION args" change and mutates cfg. The
// instruction keyword is case-insensitive, matching Docker.
func applyChange(cfg *ocispec.ImageConfig, change string) error {
	instr, args := splitInstruction(change)
	if instr == "" {
		return nil
	}
	switch strings.ToUpper(instr) {
	case "CMD":
		cfg.Cmd = execOrShellForm(args)
	case "ENTRYPOINT":
		cfg.Entrypoint = execOrShellForm(args)
	case "ENV":
		pairs, err := parseEnv(args)
		if err != nil {
			return err
		}
		for _, kv := range pairs {
			cfg.Env = setEnv(cfg.Env, kv[0], kv[1])
		}
	case "LABEL":
		pairs, err := parseLabel(args)
		if err != nil {
			return err
		}
		if cfg.Labels == nil && len(pairs) > 0 {
			cfg.Labels = map[string]string{}
		}
		for _, kv := range pairs {
			cfg.Labels[kv[0]] = kv[1]
		}
	case "EXPOSE":
		if err := parseExpose(cfg, args); err != nil {
			return err
		}
	case "VOLUME":
		vols := stringList(args)
		if cfg.Volumes == nil && len(vols) > 0 {
			cfg.Volumes = map[string]struct{}{}
		}
		for _, v := range vols {
			cfg.Volumes[v] = struct{}{}
		}
	case "USER":
		cfg.User = strings.TrimSpace(args)
	case "WORKDIR":
		cfg.WorkingDir = strings.TrimSpace(args)
	case "STOPSIGNAL":
		cfg.StopSignal = strings.TrimSpace(args)
	case "HEALTHCHECK", "ONBUILD", "SHELL":
		// These live only in Docker's image config schema, not the OCI one that
		// import writes, so they cannot be represented here.
		return fmt.Errorf("the %s instruction is not supported by import", strings.ToUpper(instr))
	default:
		return fmt.Errorf("unknown instruction %q", instr)
	}
	return nil
}

// splitInstruction splits a change into its instruction keyword and the
// remaining argument string, trimming surrounding whitespace.
func splitInstruction(change string) (instr, args string) {
	trimmed := strings.TrimSpace(change)
	i := strings.IndexAny(trimmed, " \t")
	if i < 0 {
		return trimmed, ""
	}
	return trimmed[:i], strings.TrimSpace(trimmed[i+1:])
}

// execOrShellForm parses CMD/ENTRYPOINT arguments. A valid JSON array is the
// exec form used verbatim; anything else (including a "[" that is not valid JSON)
// is the shell form, wrapped in "/bin/sh -c" the way Docker does.
func execOrShellForm(args string) []string {
	if isJSONArray(args) {
		if v, err := parseJSONStringArray(args); err == nil {
			return v
		}
	}
	if args == "" {
		return nil
	}
	return []string{"/bin/sh", "-c", args}
}

// stringList parses VOLUME arguments: a valid JSON array, or a whitespace-
// separated list of paths (also the fallback for a non-JSON "[").
func stringList(args string) []string {
	if isJSONArray(args) {
		if v, err := parseJSONStringArray(args); err == nil {
			return v
		}
	}
	return strings.Fields(args)
}

// parseExpose adds each "port[/proto]" token to cfg.ExposedPorts, defaulting the
// protocol to tcp. A "start-end" port range is expanded to one entry per port,
// matching Docker's EXPOSE.
func parseExpose(cfg *ocispec.ImageConfig, args string) error {
	for _, tok := range strings.Fields(args) {
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

// parseEnv parses ENV arguments in both forms: the legacy "ENV key value" (a
// single variable whose value is the rest of the line) and the "ENV k=v k2=v2"
// form with quote-aware values.
func parseEnv(args string) ([][2]string, error) {
	first, _ := splitInstruction(args) // reuse: first whitespace-delimited token
	if !strings.Contains(first, "=") {
		// Legacy form: first token is the key, the remainder is the value; like
		// Docker, both are required.
		key, val := splitInstruction(args)
		if key == "" || val == "" {
			return nil, fmt.Errorf("ENV must have two arguments")
		}
		return [][2]string{{key, val}}, nil
	}
	return parseKeyValuePairs(args)
}

// parseLabel parses LABEL arguments as quote-aware key=value pairs; a bare
// "key value" is accepted as a single label, matching Docker's legacy form.
func parseLabel(args string) ([][2]string, error) {
	first, _ := splitInstruction(args)
	if !strings.Contains(first, "=") {
		key, val := splitInstruction(args)
		if key == "" || val == "" {
			return nil, fmt.Errorf("LABEL must have two arguments")
		}
		return [][2]string{{key, val}}, nil
	}
	return parseKeyValuePairs(args)
}

// parseKeyValuePairs splits "k=v k2=v2" into pairs, honoring single and double
// quotes around values so a value may contain spaces.
func parseKeyValuePairs(args string) ([][2]string, error) {
	tokens, err := tokenize(args)
	if err != nil {
		return nil, err
	}
	pairs := make([][2]string, 0, len(tokens))
	for _, tok := range tokens {
		k, v, ok := strings.Cut(tok, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("expected key=value, got %q", tok)
		}
		// tokenize already strips the surrounding quotes, so v is the bare value.
		pairs = append(pairs, [2]string{k, v})
	}
	return pairs, nil
}

// tokenize splits s on whitespace that is not inside single or double quotes.
func tokenize(s string) ([]string, error) {
	var tokens []string
	var cur strings.Builder
	var quote rune
	inToken := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inToken = true
		case r == ' ' || r == '\t':
			if inToken {
				tokens = append(tokens, cur.String())
				cur.Reset()
				inToken = false
			}
		default:
			cur.WriteRune(r)
			inToken = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if inToken {
		tokens = append(tokens, cur.String())
	}
	return tokens, nil
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

// isJSONArray reports whether args looks like a JSON array (the exec form).
func isJSONArray(args string) bool {
	return strings.HasPrefix(strings.TrimSpace(args), "[")
}

// parseJSONStringArray decodes a JSON array of strings, e.g. `["echo","hi"]`.
func parseJSONStringArray(args string) ([]string, error) {
	var v []string
	if err := json.Unmarshal([]byte(args), &v); err != nil {
		return nil, fmt.Errorf("invalid JSON array %q: %w", args, err)
	}
	return v, nil
}
