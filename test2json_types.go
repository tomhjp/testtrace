package testtrace

import (
	"encoding/json"
	"time"
)

// test2JSONEvent represents a single line of output from go tool test2json. Copied
// from https://pkg.go.dev/cmd/test2json#hdr-Output_Format.
//
// When a benchmark runs, it typically produces a single line of output giving
// timing results. That line is reported in an event with Action == "output"
// and no Test field. If a benchmark logs output or reports a failure (for
// example, by using b.Log or b.Error), that extra output is reported as a
// sequence of events with Test set to the benchmark name, terminated by a final
// event with Action == "bench" or "fail". Benchmarks have no events with
// Action == "pause".
type test2JSONEvent struct {
	// Holds the time the event happened. It is conventionally omitted for cached
	// test results. Encodes as an RFC3339-format string.
	Time time.Time
	// One of a fixed set of action descriptions; see [action] constants.
	Action action
	// Specifies the package being tested. When the go command runs parallel tests
	// in -json mode, events from different tests are interlaced; the Package
	// field allows readers to separate them.
	Package string
	// Specifies the test, example, or benchmark function that caused the event.
	// Events for the overall package test do not set Test.
	Test string `json:",omitzero"`
	// Seconds. Set for "pass" and "fail" events. It gives the time elapsed for the
	// specific test or the overall package test that passed or failed.
	Elapsed float64 `json:",omitzero"`
	// Set for Action == "output" and is a portion of the test's output (standard
	// output and standard error merged together). The output is unmodified except
	// that invalid UTF-8 output from a test is coerced into valid UTF-8 by use of
	// replacement characters. With that one exception, the concatenation of the
	// Output fields of all output events is the exact output of the test execution.
	Output string `json:",omitzero"`
	// Set for Action == "fail" if the test failure was caused by a build failure.
	// It contains the package ID of the package that failed to build. This
	// matches the ImportPath field of the "go list" output, as well as the
	// BuildEvent.ImportPath field as emitted by "go build -json".
	FailedBuild string `json:",omitzero"`
}

// MarhsalJSON overrides rendering of Elapsed to include it even if it's zero
// when the Action is "pass" or "fail". This matches test2json rendering.
func (te test2JSONEvent) MarshalJSON() ([]byte, error) {
	type Alias test2JSONEvent

	var shadow struct {
		Alias
		Elapsed *float64 `json:",omitzero"`
	}

	shadow.Alias = Alias(te)

	if te.Elapsed != 0 || te.Action == actionPass || te.Action == actionFail || te.Action == actionSkip {
		shadow.Elapsed = &te.Elapsed
	}

	return json.Marshal(shadow)
}

// The action field is one of a fixed set of action descriptions.
type action string

const (
	actionStart  action = "start"  // The test binary is about to be executed.
	actionRun    action = "run"    // The test has started running.
	actionPause  action = "pause"  // The test has been paused.
	actionCont   action = "cont"   // The test has continued running.
	actionPass   action = "pass"   // The test passed.
	actionBench  action = "bench"  // The benchmark printed log output but did not fail.
	actionFail   action = "fail"   // The test or benchmark failed.
	actionOutput action = "output" // The test printed output.
	actionSkip   action = "skip"   // The test was skipped or the package contained no tests.
)
