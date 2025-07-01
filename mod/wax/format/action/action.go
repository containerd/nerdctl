package action

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

func command(out io.Writer, command string, params map[string]string, message string) {
	par := []string{}
	for k, v := range params {
		par = append(par, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(par)
	_, _ = fmt.Fprintf(out, "::%s %s::%s\n", command, strings.Join(par, ","), strings.ReplaceAll(message, "\n", "<br>")) // "%0A"))
}

func GroupStart(out io.Writer, title string) {
	command(out, "group", nil, title)
}

func GroupEnd(out io.Writer) {
	command(out, "endgroup", nil, "")
}

func Debug(out io.Writer, message string) {
	command(out, "group", nil, message)
}
