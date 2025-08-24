package report

import (
	"fmt"
	"os"
	"strings"

	"github.com/containerd/nerdctl/mod/wax/analyzer"
	"github.com/containerd/nerdctl/mod/wax/format/action"
	"github.com/containerd/nerdctl/mod/wax/gotestsum"
)

const (
	testsNotRun        = "Tests that have NOT run in any pipeline"
	testsNotRunComment = "If you authored these tests, you MUST look into this now, as this will block merging."

	testsFailingKnown        = "Tests that are known to be problematic"
	testsFailingKnownComment = "These tests are known to be flaky.\n" +
		"Failures on these are probably not related to your changeset and can usually be ignored or retried."

	testsFailingAll        = "These tests failed on ALL pipelines, and are NOT known to be problematic"
	testsFailingAllComment = "These tests are not known to be problematic.\n" +
		"The fact that they are failing on ALL pipelines is usually indicative that you broke something with your changeset." +
		"You should look into this."

	testsFailingSometimes        = "These tests failed at least once, and are NOT known to be problematic"
	testsFailingSometimesComment = "These tests are not known to be problematic.\n" +
		"If you did create these tests, then they are definitely flaky and you must look into it.\n" +
		"If you did not create these tests, and do strongly believe this failure is unrelated to your changeset, " +
		"please open a new ticket mentioning the test name in its title."
)

func Annotate(
	root string,
	version string,
	commit string,
	alwaysFailedTests []*gotestsum.FailedTest,
	failedOnceTests []*gotestsum.FailedTest,
	disabledTests []gotestsum.TestCase,
	headfile string) {

	strip := root
	if version != "" {
		strip += "/" + version
	}

	title := ""
	message := ""

	list := ""
	for _, t := range disabledTests {
		locFile, locLine, locEndLine := analyzer.FindLocation(string(t.Test), t.Package, strip)
		link := analyzer.LocationURL(commit, locFile, locLine, locEndLine, root)
		list += fmt.Sprintf("- [%s](%s)\n", t.Test, link)
	}

	if list != "" {
		title = testsNotRun
		message = fmt.Sprintf("> %s\n\n%s", testsNotRunComment, list)
		action.Error(os.Stdout, headfile, 1, 1, 0, 0, title, message)
	}

	known := ""
	unknownFailedAll := ""
	unknownFailedOnce := ""
	for _, t := range alwaysFailedTests {
		locFile, locLine, locEndLine := analyzer.FindLocation(string(t.Test), t.Package, strip)
		link := analyzer.LocationURL(commit, locFile, locLine, locEndLine, root)
		if t.KnownIssue != "" {
			spl := strings.Split(t.KnownIssue, "/")
			id := spl[len(spl)-1]
			known += fmt.Sprintf("- [%s](%s): see issue #%s\n", t.Test, link, id)
		} else {
			unknownFailedAll += fmt.Sprintf("- [%s](%s)\n", t.Test, link)
			action.Error(os.Stdout, locFile, locLine, locEndLine, 0, 0, "Test failed on all targets", "\n```\n"+strings.ReplaceAll(t.Output, "\n", "\n")+"\n```")
		}
	}

	for _, t := range failedOnceTests {
		locFile, locLine, locEndLine := analyzer.FindLocation(string(t.Test), t.Package, strip)
		link := analyzer.LocationURL(commit, locFile, locLine, locEndLine, root)
		if t.KnownIssue != "" {
			spl := strings.Split(t.KnownIssue, "/")
			id := spl[len(spl)-1]
			known += fmt.Sprintf("- [%s](%s): see issue #%s\n", t.Test, link, id)
		} else {
			unknownFailedOnce += fmt.Sprintf("- [%s](%s)\n", t.Test, link)
			action.Error(os.Stdout, locFile, locLine, locEndLine, 0, 0, "Test failed in a flaky way", "\n```\n"+strings.ReplaceAll(t.Output, "\n", "\n")+"\n```")
		}
	}

	if known != "" {
		title = testsFailingKnown
		message = fmt.Sprintf("> %s\n\n%s\n", testsFailingKnownComment, known)
		action.Warning(os.Stdout, headfile, 1, 1, 0, 0, title, message)
	}

	if unknownFailedAll != "" {
		title = testsFailingAll
		message = fmt.Sprintf("> %s\n\n%s\n", testsFailingAllComment, unknownFailedAll)
		action.Error(os.Stdout, headfile, 1, 1, 0, 0, title, message)
	}

	if unknownFailedOnce != "" {
		title = testsFailingSometimes
		message = fmt.Sprintf("> %s\n\n%s\n", testsFailingSometimesComment, unknownFailedOnce)
		action.Error(os.Stdout, headfile, 1, 1, 0, 0, title, message)
	}

}
