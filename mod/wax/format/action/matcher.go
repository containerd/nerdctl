package action

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
)

const (
	addMatcherCommand    = "add-matcher"
	removeMatcherCommand = "remove-matcher"
	permission           = 0o600
)

type matcher struct {
	ProblemMatcher *ProblemMatcher `json:"problemMatcher"`
}

type ProblemMatcherPattern struct {
	Regexp    string `json:"regexp"`
	Severity  int    `json:"severity"`
	File      int    `json:"file"`
	Line      int    `json:"line"`
	EndLine   int    `json:"endLine"`
	Column    int    `json:"column"`
	EndColumn int    `json:"endColumn"`
	Message   int    `json:"message"`
	Title     int    `json:"title"`
}

type ProblemMatcher []struct {
	Owner   string                   `json:"owner"`
	Pattern []*ProblemMatcherPattern `json:"pattern"`
}

func AddMatcher(out io.Writer, owner, regexp string, severity, endLine, file, line, title, message int) {
	mtc := &matcher{
		ProblemMatcher: &ProblemMatcher{
			{
				Owner: owner,
				Pattern: []*ProblemMatcherPattern{
					{
						Regexp:   regexp,
						Severity: severity,
						File:     file,
						Line:     line,
						EndLine:  endLine,
						Title:    title,
						Message:  message,
					},
				},
			},
		},
	}
	matcherLocation := filepath.Join(os.TempDir(), owner+".json")
	m, _ := json.MarshalIndent(mtc, "", "  ")
	_ = os.WriteFile(matcherLocation, m, permission)
	command(out, addMatcherCommand, nil, matcherLocation)
}

func RemoveMatcher(out io.Writer, owner string) {
	command(out, removeMatcherCommand, map[string]string{
		"owner": owner,
	}, "")
}
