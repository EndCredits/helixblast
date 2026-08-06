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

func BuildWhitelist(blastPath string) (*ParamWhitelist, error) {
	whitelist := &ParamWhitelist{
		params: make(map[string]bool),
	}

	programs := []string{"blastn", "blastp", "blastx", "tblastn", "tblastx"}
	composite := make(map[string]bool)

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
			composite[p] = true
		}
	}

	if len(composite) == 0 {
		return nil, fmt.Errorf("failed to parse any BLAST+ help output; check BLAST_PATH")
	}

	allowed := []string{
		"evalue", "word_size", "gapopen", "gapextend",
		"reward", "penalty", "max_target_seqs",
		"perc_identity", "qcov_hsp_perc",
		"num_threads", "strand", "task",
		"dust", "soft_masking", "lcase_masking",
		"max_hsps", "xdrop_ungap",
		"xdrop_gap", "xdrop_gap_final",
	}

	for _, a := range allowed {
		if composite[a] {
			whitelist.params[a] = true
		}
	}

	for p := range composite {
		whitelist.params[p] = true
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
