package testtrace

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io"
	"log"
	"runtime"
	"time"
)

const (
	catPkg  category = "pkg"
	catTest category = "test"
)

type TraceWriter struct {
	enc      *jsontext.Encoder
	start    time.Time
	pids     map[string]int            // maps package name -> pid
	tids     map[string]map[string]int // maps package name -> test name -> tid
	tidPools map[string]tidPool        // maps package name -> TID pool

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
		enc: jsontext.NewEncoder(w),
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
	if tw.start.Equal(time.Time{}) {
		tw.start = t2j.Time
	}

	pid, err := tw.getOrAssignPID(t2j.Package)
	if err != nil {
		return err
	}

	switch t2j.Action {
	case actionStart:
		tw.packagesRunning++
		if err := tw.addEvent(&event{
			Type:       eventTypeCounter,
			Name:       "packages",
			Categories: []category{catPkg},
			Timestamp:  t2j.Time.Sub(tw.start).Microseconds(),
			Args: map[string]any{
				"running": tw.packagesRunning,
				"passed":  tw.packagesPassed,
				"failed":  tw.packagesFailed,
				"skipped": tw.packagesSkipped,
			},
		}); err != nil {
			return err
		}
		return tw.addEvent(&event{
			Type:       eventTypeDurationStart,
			Name:       t2j.Package,
			Categories: []category{catPkg},
			Timestamp:  t2j.Time.Sub(tw.start).Microseconds(),
			ProcessID:  pid,
			ThreadID:   0,
		})
	case actionRun:
		tw.testsRunning++
		if err := tw.addEvent(&event{
			Type:       eventTypeCounter,
			Name:       "tests",
			Categories: []category{catTest},
			Timestamp:  t2j.Time.Sub(tw.start).Microseconds(),
			Args: map[string]any{
				"running": tw.testsRunning,
				"passed":  tw.testsPassed,
				"failed":  tw.testsFailed,
				"skipped": tw.testsSkipped,
			},
		}); err != nil {
			return err
		}
		tid, err := tw.getOrAssignTID(t2j.Package, t2j.Test)
		if err != nil {
			return err
		}
		return tw.addEvent(&event{
			Type:       eventTypeDurationStart,
			Name:       t2j.Test,
			Categories: []category{catTest},
			Timestamp:  t2j.Time.Sub(tw.start).Microseconds(),
			ProcessID:  pid,
			ThreadID:   tid,
		})
	case actionPass, actionFail, actionSkip:
		var (
			cat category
			tid int
		)
		if t2j.Test == "" {
			cat = catPkg
			tid = 0
			tw.packagesRunning--
			switch t2j.Action {
			case actionPass:
				tw.packagesPassed++
			case actionFail:
				tw.packagesFailed++
			case actionSkip:
				tw.packagesSkipped++
			}
			if err := tw.addEvent(&event{
				Type:       eventTypeCounter,
				Name:       "packages",
				Categories: []category{catPkg},
				Timestamp:  t2j.Time.Sub(tw.start).Microseconds(),
				Args: map[string]any{
					"running": tw.packagesRunning,
					"passed":  tw.packagesPassed,
					"failed":  tw.packagesFailed,
					"skipped": tw.packagesSkipped,
				},
			}); err != nil {
				return err
			}
		} else {
			cat = catTest
			tid, err = tw.releaseTID(t2j.Package, t2j.Test)
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
			if err := tw.addEvent(&event{
				Type:       eventTypeCounter,
				Name:       "tests",
				Categories: []category{catTest},
				Timestamp:  t2j.Time.Sub(tw.start).Microseconds(),
				Args: map[string]any{
					"running": tw.testsRunning,
					"passed":  tw.testsPassed,
					"failed":  tw.testsFailed,
					"skipped": tw.testsSkipped,
				},
			}); err != nil {
				return err
			}
		}
		if t2j.Action == actionFail && t2j.Test != "" {
			if err := tw.addEvent(&event{
				Type:      eventTypeInstant,
				Name:      fmt.Sprintf("%s FAILED", t2j.Test),
				Scope:     scopeThread,
				Timestamp: t2j.Time.Sub(tw.start).Microseconds(),
				ProcessID: pid,
				ThreadID:  tid,
			}); err != nil {
				return err
			}
		}
		return tw.addEvent(&event{
			Type:       eventTypeDurationEnd,
			Categories: []category{cat},
			Timestamp:  t2j.Time.Sub(tw.start).Microseconds(),
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
	}

	return nil
}

func (tw *TraceWriter) getOrAssignPID(pkg string) (int, error) {
	if pid, ok := tw.pids[pkg]; ok {
		return pid, nil
	}

	if tw.pids == nil {
		tw.pids = make(map[string]int)
	}
	pid := 1 + len(tw.pids)
	tw.pids[pkg] = pid

	// Now emit metadata events to affect how the new package's PID is displayed.
	if err := tw.addEvent(&event{
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
	if err := tw.addEvent(&event{
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

func (tw *TraceWriter) getOrAssignTID(pkg, test string) (int, error) {
	if tw.tids == nil {
		tw.tids = make(map[string]map[string]int)
	}
	if tw.tids[pkg] == nil {
		tw.tids[pkg] = make(map[string]int)
	}

	pid, err := tw.getOrAssignPID(pkg)
	if err != nil {
		return 0, err
	}

	if tid, ok := tw.tids[pkg][test]; ok {
		return tid, nil
	}

	if tw.tidPools == nil {
		tw.tidPools = make(map[string]tidPool)
	}
	if tw.tidPools[pkg] == nil {
		tw.tidPools[pkg] = tidPool{}
	}

	tid := tw.tidPools[pkg].lowestAvailable()
	tw.tids[pkg][test] = tid
	log.Printf("assigning %d for %s %s", tid, pkg, test)

	// Now emit metadata event to affect how the TID is displayed.
	if err := tw.addEvent(&event{
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

	return tid, nil
}

func (tw *TraceWriter) releaseTID(pkg, test string) (int, error) {
	tid, err := tw.getOrAssignTID(pkg, test)
	if err != nil {
		return 0, err
	}
	tw.tidPools[pkg].release(tid)
	delete(tw.tids[pkg], test)
	return tid, nil
}

func (tw *TraceWriter) addEvent(e *event) error {
	return json.MarshalEncode(tw.enc, e)
}

type tidPool map[int]struct{}

func (tp tidPool) lowestAvailable() int {
	for tid := 1; ; tid++ {
		if _, taken := tp[tid]; !taken {
			tp[tid] = struct{}{}
			return tid
		}
	}
}

func (tp tidPool) release(tid int) {
	delete(tp, tid)
}
