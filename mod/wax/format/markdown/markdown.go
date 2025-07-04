package markdown

import (
	"fmt"
	"io"
	"strings"
)

func markdown(out io.Writer, prefix, message, suffix string) {
	_, _ = fmt.Fprintf(out, "%s %s %s\n", prefix, message, suffix)
}

func H1(out io.Writer, message string) {
	markdown(out, "#", message, "")
}

func H2(out io.Writer, message string) {
	markdown(out, "##", message, "")
}

func H3(out io.Writer, message string) {
	markdown(out, "###", message, "")
}

func Blockquote(out io.Writer, message string) {
	markdown(out, ">", message, "")
}

func Table(out io.Writer, headers []string, rows [][]string) {
	markdown(out, "|", strings.Join(headers, "|"), "|")
	markdown(out, "|", strings.Repeat(" ----- |", len(headers)), "")
	for _, row := range rows {
		markdown(out, "|", strings.Join(row, "|"), "|")
	}
}

func Pie(out io.Writer, title string, values map[string]string) {
	markdown(out, "```", "mermaid", "")
	markdown(out, "", "pie", "")
	markdown(out, "title", title, "")
	for label, value := range values {
		markdown(out, fmt.Sprintf("%q", label), value, "")
	}
	markdown(out, "", "", "```")
}
