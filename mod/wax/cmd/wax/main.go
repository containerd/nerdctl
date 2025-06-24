package main

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"gotest.tools/gotestsum/testjson"
	"io"
	"os"
	"time"
)

type GoTestSumReport struct {
}

func main() {
	err := run(&options{
		threshold: time.Millisecond,
		topN:      2,
		jsonfile:  "/Users/dmp/Projects/go/nerd/nerdctl/./mod/wax/cmd/wax/test-integration.log",
		debug:     true,
	})
	fmt.Println(err)
	//testjson.TestEvent{}
}

type options struct {
	threshold     time.Duration
	topN          int
	jsonfile      string
	skipStatement string
	debug         bool
}

func jsonfileReader(v string) (io.ReadCloser, error) {
	switch v {
	case "", "-":
		return io.NopCloser(os.Stdin), nil
	default:
		return os.Open(v)
	}
}

func run(opts *options) error {
	log.SetLevel(log.DebugLevel)

	in, err := jsonfileReader(opts.jsonfile)
	if err != nil {
		return fmt.Errorf("failed to read jsonfile: %v", err)
	}
	defer func() {
		if err := in.Close(); err != nil {
			log.Errorf("Failed to close file %v: %v", opts.jsonfile, err)
		}
	}()

	exec, err := testjson.ScanTestOutput(testjson.ScanConfig{Stdout: in})
	if err != nil {
		return fmt.Errorf("failed to scan testjson: %v", err)
	}

	pkgs := exec.Packages()
	for _, pkg := range pkgs {
		fmt.Println(exec.Package(pkg).TestCases())
	}

	return nil
	//tcs := Slowest(exec, opts.threshold, opts.topN)
	//if opts.skipStatement != "" {
	//	skipStmt, err := parseSkipStatement(opts.skipStatement)
	//	if err != nil {
	//		return fmt.Errorf("failed to parse skip expr: %v", err)
	//	}
	//	return writeTestSkip(tcs, skipStmt)
	//}
	//for _, tc := range tcs {
	//	fmt.Printf("%s %s %v\n", tc.Package, tc.Test, tc.Elapsed)
	//}

	//return nil
}

// Slowest returns a slice of all tests with an elapsed time greater than
// threshold. The slice is sorted by Elapsed time in descending order (slowest
// test first).
//
// If there are multiple runs of a TestCase, all of them will be represented
// by a single TestCase with the median elapsed time in the returned slice.
//func Slowest(exec *testjson.Execution, threshold time.Duration, num int) []testjson.TestCase {
//if threshold == 0 && num == 0 {
//	return nil
//}
//pkgs := exec.Packages()
//tests := make([]testjson.TestCase, 0, len(pkgs))
//for _, pkg := range pkgs {
//	pkgTests := ByElapsed(exec.Package(pkg).TestCases(), median)
//	tests = append(tests, pkgTests...)
//}
//sort.Slice(tests, func(i, j int) bool {
//	return tests[i].Elapsed > tests[j].Elapsed
//})
//if num >= len(tests) {
//	return tests
//}
//if num > 0 {
//	return tests[:num]
//}
//
//end := sort.Search(len(tests), func(i int) bool {
//	return tests[i].Elapsed < threshold
//})
//return tests[:end]
//}

// ByElapsed maps all test cases by name, and if there is more than one
// instance of a TestCase, uses fn to select the elapsed time for the group.
//
// All cases are assumed to be part of the same package.
//func ByElapsed(cases []testjson.TestCase, fn func(times []time.Duration) time.Duration) []testjson.TestCase {
//	if len(cases) <= 1 {
//		return cases
//	}
//	pkg := cases[0].Package
//	m := make(map[testjson.TestName][]time.Duration)
//	for _, tc := range cases {
//		m[tc.Test] = append(m[tc.Test], tc.Elapsed)
//	}
//	result := make([]testjson.TestCase, 0, len(m))
//	for name, timing := range m {
//		result = append(result, testjson.TestCase{
//			Package: pkg,
//			Test:    name,
//			Elapsed: fn(timing),
//		})
//	}
//	return result
//}
//
//func median(times []time.Duration) time.Duration {
//	switch len(times) {
//	case 0:
//		return 0
//	case 1:
//		return times[0]
//	}
//	sort.Slice(times, func(i, j int) bool {
//		return times[i] < times[j]
//	})
//	return times[len(times)/2]
//}
//
//func writeFile(path string, file *ast.File, fset *token.FileSet) error {
//	fh, err := os.Create(path)
//	if err != nil {
//		return err
//	}
//	defer func() {
//		if err := fh.Close(); err != nil {
//			log.Errorf("Failed to close file %v: %v", path, err)
//		}
//	}()
//	return format.Node(fh, fset, file)
//}
//
//func parseSkipStatement(text string) (ast.Stmt, error) {
//	switch text {
//	case "default", "testing.Short":
//		text = `
//	if testing.Short() {
//		t.Skip("too slow for testing.Short")
//	}
//`
//	}
//	// Add some required boilerplate around the statement to make it a valid file
//	text = "package stub\nfunc Stub() {\n" + text + "\n}\n"
//	file, err := parser.ParseFile(token.NewFileSet(), "fragment", text, 0)
//	if err != nil {
//		return nil, err
//	}
//	stmt := file.Decls[0].(*ast.FuncDecl).Body.List[0]
//	return stmt, nil
//}
