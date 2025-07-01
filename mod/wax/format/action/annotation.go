package action

import (
	"io"
	"strconv"
)

const (
	warningLevel = "warning"
	noticeLevel  = "notice"
	errorLevel   = "error"
	fileKey      = "file"
	columnKey    = "col"
	endColumnKey = "endColumn"
	lineKey      = "line"
	endLineKey   = "endLine"
	titleKey     = "title"
)

func annotation(out io.Writer, level, file string, col, endColumn, line, endLine int, title, message string) {
	params := map[string]string{}

	if file != "" {
		params[fileKey] = file
	}

	if col != 0 {
		params[columnKey] = strconv.Itoa(col)
	}

	if line != 0 {
		params[lineKey] = strconv.Itoa(line)
	}

	if endColumn != 0 {
		params[endColumnKey] = strconv.Itoa(endColumn)
	}

	if endLine != 0 {
		params[endLineKey] = strconv.Itoa(endLine)
	}

	if title != "" {
		params[titleKey] = title
	}

	command(out, level, params, message)
}

func Notice(out io.Writer, file string, line, endLine, col, endColumn int, title, message string) {
	annotation(out, noticeLevel, file, col, endColumn, line, endLine, title, message)
}

func Warning(out io.Writer, file string, line, endLine, col, endColumn int, title, message string) {
	annotation(out, warningLevel, file, col, endColumn, line, endLine, title, message)
}

func Error(out io.Writer, file string, line, endLine, col, endColumn int, title, message string) {
	annotation(out, errorLevel, file, col, endColumn, line, endLine, title, message)
}
