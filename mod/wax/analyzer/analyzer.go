package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func FindLocation(testName string, packageName string, base string) (string, int, int) {
	root := packageName[len(base)+1:]
	topTestName := strings.Split(testName, "/")[0]
	foundFile := ""
	foundLine := 0
	foundEndLine := 0

	err := filepath.WalkDir(root, func(path string, info os.DirEntry, err error) error {
		if foundFile != "" {
			return nil
		}

		if err != nil {
			return err
		}

		l, _ := os.Stat(path)
		if !l.IsDir() && strings.HasSuffix(filepath.Base(path), "_test.go") {
			cc, _ := os.ReadFile(path)
			for id, line := range strings.Split(string(cc), "\n") {
				if line == fmt.Sprintf("func %s(t *testing.T) {", topTestName) {
					foundFile = path
					foundLine = id + 1
				} else if foundFile != "" && line == "}" {
					foundEndLine = id + 1
					break
				}
			}
		}

		return nil
	})

	if err != nil {
		fmt.Println(err)
	}

	return foundFile, foundLine, foundEndLine
}

func LocationURL(commit, file string, start, end int, base string) string {
	return fmt.Sprintf("https://%s/blob/%s/%s#L%d-L%d", base, commit, file, start, end)
}
