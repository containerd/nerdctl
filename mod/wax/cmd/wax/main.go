package main

import (
	"context"
	"github.com/containerd/nerdctl/mod/wax/format/action"
	"os"
	"os/exec"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/containerd/nerdctl/mod/wax/api"
	"github.com/containerd/nerdctl/mod/wax/gotestsum"
	"github.com/containerd/nerdctl/mod/wax/report"
)

func main() {
	//mainLine := "::error endLine=84,file=cmd/nerdctl/ipfs/ipfs_compose_linux_test.go,line=41,title=Test failed in a flaky way::%0A```%0A=== RUN   TestIPFSCompNoBuild%0A=== PAUSE TestIPFSCompNoBuild%0A=== CONT\n"
	////rgx := "^::([^:]+) file=([^,:]+),line=(\\d+)(?:,endLine=(\\d+))?,title=([^,:]+)::WAX-MARK-(.*)-WAX-MARK$"

	//rgx := "(?m)^::([^:]+) (?:endLine=(\\d+),)?file=([^,:]+),line=(\\d+),title=([^,:]+)::(.*)"
	//r := regexp.MustCompile(rgx)
	//res := r.FindStringSubmatch(mainLine)
	//fmt.Println("LLALA")
	//fmt.Println(">>>", res)
	//return

	tk := os.Getenv("GITHUB_TOKEN")
	sha := os.Getenv("GITHUB_SHA")
	rep := strings.Split(os.Getenv("GITHUB_REPOSITORY"), "/")
	gh := api.New(tk)
	issues, err := gh.Issues(context.Background(), api.Open, rep[0], rep[1])
	if err != nil {
		log.Fatal(err)
	}

	reportsRoot := os.Args[1]

	run, _ := gotestsum.FromLogs(gotestsum.FindLogs(reportsRoot, "test-integration")...)
	ls, _ := exec.Command("git", "diff", "--name-only", "-r", "HEAD^1", "HEAD").CombinedOutput()
	changedFiles := strings.Split(string(ls), "\n")
	topfile := changedFiles[0]

	alwaysFailingTests := run.FailedAlways()
	for _, failedTest := range alwaysFailingTests {
		var found *api.Issue
		// Look for the top-level test, not subtests
		compare := strings.Split(string(failedTest.Test), "/")[0]
		for _, issue := range issues {
			if strings.Contains(*issue.Title, compare) {
				found = issue
				break
			}
		}
		if found != nil {
			failedTest.KnownIssue = *found.HTMLURL
		}
	}

	onceFailingTests := run.FailedAtLeastOnceButNotAlways()
	for _, failedTest := range onceFailingTests {
		var found *api.Issue
		// Look for the top-level test, not subtests
		compare := strings.Split(string(failedTest.Test), "/")[0]
		for _, issue := range issues {
			if strings.Contains(*issue.Title, compare) {
				found = issue
				break
			}
		}
		if found != nil {
			failedTest.KnownIssue = *found.HTMLURL
		}
	}

	testsThatNeverRan := run.NeverRan()

	// ::error file=.github/matchers/wax.json,line=1,endLine=1,title=Tests that have NOT run in any pipeline::WAX-MARK-

	//"regexp": "^([^:]+):(\\d+):(\\d+):\\s+(error|warning):\\s+(.*)$",
	//
	//action.AddMatcher(os.Stdout, "wax", `^(\w+):\s+(\d+)\s+(\d+)\s+(.+)$`, 1, 2, 3, 4, 5)

	action.AddMatcher(os.Stdout, "wax", "^::([^:]+) (?:endLine=(\\\\d+),)?file=([^,:]+),line=(\\\\d+),title=([^,:]+)::(.*)$", 1, 3, 4, 2, 5, 6)
	report.Annotate("github.com/"+strings.Join(rep, "/"), "v2", sha, alwaysFailingTests, onceFailingTests, testsThatNeverRan, topfile)
	action.RemoveMatcher(os.Stdout, "wax")
}
