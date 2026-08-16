package testtrace

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"time"
)

const (
	catPkg  category = "pkg"
	catTest category = "test"
	runPID           = 1 // The "PID" for the parent process of the whole test suite.
)

type (
	processID int
	threadID  int
	pkgName   string
	testName  string
)

func (t testName) parent() (testName, bool) {
	i := strings.LastIndexByte(string(t), '/')
	if i < 0 {
		return "", false
	}
	return t[:i], true
}

type TraceWriter struct {
	enc     *jsontext.Encoder
	start   time.Time
	nextPID processID
	pids    map[pkgName]processID
	tids    map[processID]*tids

	// lastTimestamp is the timestamp of the most recently recorded event, and is
	// used as the default time when test2json omits a timestamp (e.g. for cached
	// results).
	lastTimestamp microseconds

	packagesRunning int
	packagesPassed  int
	packagesFailed  int
	packagesSkipped int

	testsRunning int
	testsPassed  int
	testsFailed  int
	testsSkipped int
}

type TraceWriterOption func(*TraceWriter)

func NewTraceWriter(w io.Writer, opts ...TraceWriterOption) (*TraceWriter, error) {
	tw := &TraceWriter{
		enc:     jsontext.NewEncoder(w),
		nextPID: runPID + 1,
	}

	for _, opt := range opts {
		opt(tw)
	}

	if err := tw.enc.WriteToken(jsontext.BeginObject); err != nil {
		return nil, err
	}
	if err := tw.enc.WriteToken(jsontext.String("traceEvents")); err != nil {
		return nil, err
	}
	if err := tw.enc.WriteToken(jsontext.BeginArray); err != nil {
		return nil, err
	}

	return tw, nil
}

func (tw *TraceWriter) Close() error {
	if err := tw.enc.WriteToken(jsontext.EndArray); err != nil {
		return err
	}
	// Metadata for the trace. They will be collected and stored in an array in
	// the trace model. This metadata is accessible through the Metadata button
	// in Trace Viewer.
	metadata := map[string]any{
		"goos":   runtime.GOOS,
		"goarch": runtime.GOARCH,
	}
	if err := tw.enc.WriteToken(jsontext.String("metadata")); err != nil {
		return fmt.Errorf("error writing metadata key: %w", err)
	}
	if err := json.MarshalEncode(tw.enc, metadata); err != nil {
		return fmt.Errorf("error writing metadata object: %w", err)
	}
	if err := tw.enc.WriteToken(jsontext.EndObject); err != nil {
		return err
	}

	return nil
}

func (tw *TraceWriter) AddTest2JSONLine(line []byte) error {
	var t2j test2JSONEvent
	if err := json.Unmarshal(line, &t2j); err != nil {
		return err
	}
	if tw.start.IsZero() {
		tw.start = t2j.Time
	}

	pid, err := tw.pidFor(t2j.Package)
	if err != nil {
		return err
	}

	if t2j.Test == "" {
		return tw.handlePackage(&t2j, pid)
	}

	return tw.handleTest(&t2j, pid)
}

func (tw *TraceWriter) handlePackage(t2j *test2JSONEvent, pid processID) error {
	ts := tw.timestampFor(t2j)
	switch t2j.Action {
	case actionStart:
		tw.packagesRunning++
		if err := tw.emitPackagesCounter(ts); err != nil {
			return err
		}
		return tw.emit(&event{
			Type:       eventTypeDurationStart,
			Name:       string(t2j.Package),
			Categories: []category{catPkg},
			Timestamp:  ts,
			ProcessID:  pid,
			ThreadID:   0,
		})
	case actionPass, actionFail, actionSkip:
		cat := catPkg
		tw.packagesRunning--
		switch t2j.Action {
		case actionPass:
			tw.packagesPassed++
		case actionFail:
			tw.packagesFailed++
		case actionSkip:
			tw.packagesSkipped++
		}
		if err := tw.emitPackagesCounter(ts); err != nil {
			return err
		}
		return tw.emit(&event{
			Type:       eventTypeDurationEnd,
			Categories: []category{cat},
			Timestamp:  ts,
			ProcessID:  pid,
			ThreadID:   0,
			Args: func() map[string]any {
				m := map[string]any{
					"result": t2j.Action,
				}
				if t2j.Elapsed != 0 {
					// Elapsed time as reported by go.
					m["elapsed_ms"] = t2j.Elapsed * 1000
				}
				return m
			}(),
		})
	case actionPause, actionCont:
		return nil
	case actionBench:
	case actionOutput:
	default:
		return fmt.Errorf("unexpected %q action for package event: %+v", t2j.Action, t2j)
	}

	return nil
}

func (tw *TraceWriter) handleTest(t2j *test2JSONEvent, pid processID) error {
	ts := tw.timestampFor(t2j)
	switch t2j.Action {
	case actionRun:
		tw.testsRunning++
		if err := tw.emitTestsCounter(ts); err != nil {
			return err
		}
		tid, err := tw.tidFor(pid, t2j.Test)
		if err != nil {
			return err
		}
		return tw.emit(&event{
			Type:       eventTypeDurationStart,
			Name:       string(t2j.Test),
			Categories: []category{catTest},
			Timestamp:  ts,
			ProcessID:  pid,
			ThreadID:   tid,
		})
	case actionPass, actionFail, actionSkip:
		cat := catTest
		tid, err := tw.closeTID(pid, t2j.Test)
		if err != nil {
			return err
		}
		tw.testsRunning--
		switch t2j.Action {
		case actionPass:
			tw.testsPassed++
		case actionFail:
			tw.testsFailed++
		case actionSkip:
			tw.testsSkipped++
		}
		if err := tw.emitTestsCounter(ts); err != nil {
			return err
		}
		if t2j.Action == actionFail {
			if err := tw.emit(&event{
				Type:      eventTypeInstant,
				Name:      fmt.Sprintf("%s FAILED", t2j.Test),
				Scope:     scopeThread,
				Timestamp: ts,
				ProcessID: pid,
				ThreadID:  tid,
			}); err != nil {
				return err
			}
		}
		return tw.emit(&event{
			Type:       eventTypeDurationEnd,
			Categories: []category{cat},
			Timestamp:  ts,
			ProcessID:  pid,
			ThreadID:   tid,
			Args: map[string]any{
				"result": t2j.Action,
			},
		})
	case actionPause, actionCont:
		return nil
	case actionBench:
	case actionOutput:
	default:
		return fmt.Errorf("unexpected %q action for test event: %+v", t2j.Action, t2j)
	}

	return nil
}

func (tw *TraceWriter) pidFor(pkg pkgName) (processID, error) {
	if pid, ok := tw.pids[pkg]; ok {
		return pid, nil
	}

	if tw.pids == nil {
		tw.pids = make(map[pkgName]processID)
	}
	pid := tw.nextPID
	tw.nextPID++
	tw.pids[pkg] = pid

	// Now emit metadata events to affect how the new package's PID is displayed.
	if err := tw.emit(&event{
		Type:      eventTypeMetadata,
		Name:      metadataProcessName,
		ProcessID: pid,
		ThreadID:  0,
		Args: map[string]any{
			"name": pkg,
		},
	}); err != nil {
		return 0, err
	}
	if err := tw.emit(&event{
		Type:      eventTypeMetadata,
		Name:      metadataThreadName,
		ProcessID: pid,
		ThreadID:  0,
		Args: map[string]any{
			"name": "go test " + pkg,
		},
	}); err != nil {
		return 0, err
	}

	return pid, nil
}

func (tw *TraceWriter) tidFor(pid processID, test testName) (threadID, error) {
	if tw.tids == nil {
		tw.tids = make(map[processID]*tids)
	}
	if tw.tids[pid] == nil {
		tw.tids[pid] = new(tids)
	}

	tid, opened := tw.tids[pid].getOrOpenTID(test)
	if opened {
		// Now emit metadata event to affect how the TID is displayed.
		if err := tw.emit(&event{
			Type:      eventTypeMetadata,
			Name:      metadataThreadName,
			ProcessID: pid,
			ThreadID:  tid,
			Args: map[string]any{
				"name": "lane",
			},
		}); err != nil {
			return 0, err
		}
	}

	return tid, nil
}

func (tw *TraceWriter) closeTID(pid processID, test testName) (threadID, error) {
	tid := tw.tids[pid].getTID(test)
	if tid == 0 {
		return 0, fmt.Errorf("tried to close span for test %q but it didn't have a start", test)
	}
	tw.tids[pid].closeTID(test)
	return tid, nil
}

func (tw *TraceWriter) emitPackagesCounter(ts microseconds) error {
	// TODO: coalesce counter events to some reasonable maximum rate like 100Hz.
	return tw.emit(&event{
		Type:       eventTypeCounter,
		Name:       "packages",
		Categories: []category{catPkg},
		Timestamp:  ts,
		ProcessID:  runPID,
		ThreadID:   0,
		Args: map[string]any{
			"running": tw.packagesRunning,
			"passed":  tw.packagesPassed,
			"failed":  tw.packagesFailed,
			"skipped": tw.packagesSkipped,
		},
	})
}

func (tw *TraceWriter) emitTestsCounter(ts microseconds) error {
	// TODO: coalesce counter events to some reasonable maximum rate like 100Hz.
	return tw.emit(&event{
		Type:       eventTypeCounter,
		Name:       "tests",
		Categories: []category{catPkg},
		Timestamp:  ts,
		ProcessID:  runPID,
		ThreadID:   0,
		Args: map[string]any{
			"running": tw.testsRunning,
			"passed":  tw.testsPassed,
			"failed":  tw.testsFailed,
			"skipped": tw.testsSkipped,
		},
	})
}

func (tw *TraceWriter) emit(e *event) error {
	return json.MarshalEncode(tw.enc, e)
}

func (tw *TraceWriter) timestampFor(t2j *test2JSONEvent) microseconds {
	if t2j.Time.IsZero() {
		return tw.lastTimestamp
	}
	ts := microseconds(t2j.Time.Sub(tw.start).Microseconds())
	tw.lastTimestamp = ts
	return ts
}

// tids is the metadata for all tests of a single test package.
type tids struct {
	assigned map[testName]threadID   // All assigned thread IDs across the test package lifetime.
	open     map[threadID][]testName // The stack of currently running tests per thread, innermost last.
}

func (t *tids) getTID(test testName) threadID {
	if t.assigned == nil {
		t.assigned = make(map[testName]threadID)
	}
	if t.open == nil {
		t.open = make(map[threadID][]testName)
	}

	if tid, ok := t.assigned[test]; ok {
		return tid
	}

	return 0
}

func (t *tids) getOrOpenTID(test testName) (threadID, bool) {
	if tid := t.getTID(test); tid != 0 {
		t.open[tid] = append(t.open[tid], test)
		return tid, false
	}

	// A sub-test shares its parent's thread if the parent is the innermost span
	// currently open there; that keeps each thread's begin/end events strictly
	// nested even when sub-tests run in parallel.
	if parent, ok := test.parent(); ok {
		if tid, ok := t.assigned[parent]; ok {
			if stack := t.open[tid]; len(stack) > 0 && stack[len(stack)-1] == parent {
				t.assigned[test] = tid
				t.open[tid] = append(stack, test)
				return tid, false
			}
		}
	}

	for tid := threadID(1); ; tid++ {
		if len(t.open[tid]) == 0 {
			t.assigned[test] = tid
			t.open[tid] = append(t.open[tid], test)
			return tid, true
		}
	}
}

func (t *tids) closeTID(test testName) {
	tid := t.assigned[test]
	stack := t.open[tid]
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i] == test {
			stack = slices.Delete(stack, i, i+1)
			break
		}
	}
	if len(stack) == 0 {
		delete(t.open, tid)
	} else {
		t.open[tid] = stack
	}
}
