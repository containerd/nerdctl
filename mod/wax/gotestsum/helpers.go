package gotestsum

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func testIntersection(slice1, slice2 []*FailedTest) []*FailedTest {
	seen := make(map[string]bool)
	result := []*FailedTest{}

	for _, val1 := range slice1 {
		tn1 := val1.Package + "/" + string(val1.Test)
		for _, val2 := range slice2 {
			tn2 := val2.Package + "/" + string(val2.Test)
			if tn1 == tn2 {
				if _, ok := seen[tn1]; !ok {
					seen[tn1] = true
					result = append(result, val1)
					break
				}
			}
		}
	}

	return result
}

func testTestIntersection(slice1, slice2 []TestCase) []TestCase {
	seen := make(map[string]bool)
	result := []TestCase{}

	for _, val1 := range slice1 {
		tn1 := val1.Package + "/" + string(val1.Test)
		for _, val2 := range slice2 {
			tn2 := val2.Package + "/" + string(val2.Test)
			if tn1 == tn2 {
				if _, ok := seen[tn1]; !ok {
					seen[tn1] = true
					result = append(result, val1)
					break
				}
			}
		}
	}

	return result
}

func testUnion(slice1, slice2 []*FailedTest) []*FailedTest {
	seen := make(map[string]bool)
	result := []*FailedTest{}

	for _, val := range slice1 {
		tn := val.Package + "/" + string(val.Test)
		if _, ok := seen[tn]; !ok {
			seen[tn] = true
			result = append(result, val)
		}
	}

	for _, val := range slice2 {
		tn := val.Package + "/" + string(val.Test)
		if _, ok := seen[tn]; !ok {
			seen[tn] = true
			result = append(result, val)
		}
	}

	return result
}

func testNotIn(slice1, slice2 []*FailedTest) []*FailedTest {
	mb := make(map[string]bool, len(slice1))
	for _, val := range slice1 {
		tn := val.Package + "/" + string(val.Test)
		mb[tn] = true
	}
	var diff []*FailedTest
	for _, val := range slice2 {
		tn := val.Package + "/" + string(val.Test)
		if _, found := mb[tn]; !found {
			diff = append(diff, val)
		}
	}
	return diff
}

func findFiles(root, needle string) []string {
	files := []string{}
	err := filepath.WalkDir(root, func(path string, info os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		l, _ := os.Stat(path)
		if l.IsDir() && path != root {
			files = append(files, findFiles(path, needle)...)
		} else if strings.HasPrefix(filepath.Base(path), needle) {
			files = append(files, path)
		}

		return nil
	})

	if err != nil {
		fmt.Println(err)
	}

	return files
}
