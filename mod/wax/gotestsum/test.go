package gotestsum

import "gotest.tools/gotestsum/testjson"

type TestCase = testjson.TestCase

type FailedTest struct {
	TestCase
	KnownIssue   string
	Output       string
	AlwaysFailed bool
}
