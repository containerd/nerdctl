package main

import "testing"

func TestSkipOne(t *testing.T) {
	t.Skip("skip one")
}

func TestFailOne(t *testing.T) {
	t.FailNow()
}

func TestSuccess(t *testing.T) {
	return
}
