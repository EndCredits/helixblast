package blast

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

type ParamWhitelist struct {
	params map[string]bool
}

// reservedParams are injected by the server itself in ExecConfig.BuildCommand.
// BLAST+ applies last-wins semantics for duplicated flags, so an advanced param
// of the same name appended later would silently override server-controlled
// behavior (query file, target database, output format, thread budget, output
// capture). They are excluded from the whitelist.
var reservedParams = map[string]bool{
	"query":       true,
	"db":          true,
	"outfmt":      true,
	"num_threads": true,
	"out":         true,
}

func BuildWhitelist(blastPath string) (*ParamWhitelist, error) {
	whitelist := &ParamWhitelist{
		params: make(map[string]bool),
	}

	programs := []string{"blastn", "blastp", "blastx", "tblastn", "tblastx"}

	for _, prog := range programs {
		exe := prog
		if blastPath != "" {
			exe = blastPath + "/" + prog
		}
		progParams, err := parseHelpFlags(exe)
		if err != nil {
			continue
		}
		for _, p := range progParams {
			if !reservedParams[p] {
				whitelist.params[p] = true
			}
		}
	}

	if len(whitelist.params) == 0 {
		return nil, fmt.Errorf("failed to parse any BLAST+ help output; check BLAST_PATH")
	}

	return whitelist, nil
}

func (pw *ParamWhitelist) IsAllowed(param string) bool {
	return pw.params[param]
}

func (pw *ParamWhitelist) Len() int {
	return len(pw.params)
}

// NewParamWhitelist returns a whitelist containing exactly the given keys.
// Useful for tests and programmatic construction where running `-help` on
// real BLAST binaries is not possible.
func NewParamWhitelist(keys []string) *ParamWhitelist {
	pw := &ParamWhitelist{params: make(map[string]bool, len(keys))}
	for _, k := range keys {
		pw.params[k] = true
	}
	return pw
}

var helpFlagRe = regexp.MustCompile(`(?m)^\s*-(?:\w*-)?(\w[\w-]*)`)

func parseHelpFlags(executable string) ([]string, error) {
	cmd := exec.Command(executable, "-help")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exec %s -help: %w", executable, err)
	}

	seen := make(map[string]bool)
	matches := helpFlagRe.FindAllStringSubmatch(string(out), -1)
	for _, m := range matches {
		if len(m) > 1 {
			name := strings.ToLower(m[1])
			if len(name) < 2 || name == "h" || name == "help" || name == "version" {
				continue
			}
			seen[name] = true
		}
	}

	result := make([]string, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	return result, nil
}
