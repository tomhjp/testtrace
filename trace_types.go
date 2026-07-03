package testtrace

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Spec: https://docs.google.com/document/d/1CvAClvFfyA5R-PhYUmn5OOQtYMH4h6I0nSsKchNAySU

// A single event from traceEvents, as specified at:
// https://docs.google.com/document/d/1CvAClvFfyA5R-PhYUmn5OOQtYMH4h6I0nSsKchNAySU/edit?tab=t.0#heading=h.uxpopqvbjezh
type event struct {
	// The name of the event, as displayed in Trace Viewer.
	Name string `json:"name,omitzero"`
	// The event categories. This is a comma separated list of categories for the
	// event. The categories can be used to hide events in the Trace Viewer UI.
	Categories []category `json:"cat,omitzero"`
	// The event type. This is a single character which changes depending on the
	// type of event being output.
	Type eventType `json:"ph"`
	// The tracing clock timestamp of the event. The timestamps are provided at
	// microsecond granularity.
	Timestamp int64 `json:"ts"`
	// Optional. The thread clock timestamp of the event. The timestamps are
	// provided at microsecond granularity.
	ThreadTimestamp uint64 `json:"tts,omitzero"`
	// The process ID for the process that output this event.
	ProcessID int `json:"pid"`
	// The thread ID for the thread that output this event.
	ThreadID int `json:"tid"`
	// Any arguments provided for the event. Some of the event types have required
	// argument fields, otherwise, you can put any information you wish in here.
	// The arguments are displayed in Trace Viewer when you view an event in the
	// analysis section.
	Args map[string]any `json:"args"`
	// Optional. A fixed color name to associate with the event. If provided, cname
	// must be one of the names listed in trace-viewer's base color scheme's
	// reserved color names list.
	ColorName string `json:"cname"`
	// Scope specifies the scope of the event, and is unique to Instant events.
	// There are four scopes available global (g), process (p) and thread (t). If
	// no scope is provided we default to thread scoped events.
	Scope scope `json:"s,omitzero"`
}

type eventType string

const (
	eventTypeDurationStart     eventType = "B"
	eventTypeDurationEnd       eventType = "E"
	eventTypeComplete          eventType = "X"
	eventTypeInstant           eventType = "i"
	eventTypeCounter           eventType = "C"
	eventTypeAsyncStart        eventType = "b"
	eventTypeAsyncInstant      eventType = "n"
	eventTypeAsyncEnd          eventType = "e"
	eventTypeFlowStart         eventType = "s"
	eventTypeFlowStep          eventType = "t"
	eventTypeFlowEnd           eventType = "f"
	eventTypeSample            eventType = "P"
	eventTypeObjectCreated     eventType = "N"
	eventTypeObjectSnapshot    eventType = "O"
	eventTypeObjectDestroyed   eventType = "D"
	eventTypeMetadata          eventType = "M"
	eventTypeMemoryDumpGlobal  eventType = "V"
	eventTypeMemoryDumpProcess eventType = "v"
	eventTypeMark              eventType = "R"
	eventTypeClockSync         eventType = "c"
	eventTypeContext           eventType = ","
)

// A category is used for filtering
type category string

const (
	// There are 5 types of Metadata event, see:
	// https://docs.google.com/document/d/1CvAClvFfyA5R-PhYUmn5OOQtYMH4h6I0nSsKchNAySU/edit?tab=t.0#bookmark=id.iycbnb4z7i9g
	metadataProcessName      = "process_name"       // Sets the display name for the provided pid. The name is provided in a name argument.
	metadataProcessLabels    = "process_labels"     // Sets the extra process labels for the provided pid. The label is provided in a labels argument.
	metadataProcessSortIndex = "process_sort_index" // Sets the process sort order position. The sort index is provided in a sort_index argument.
	metadataThreadName       = "thread_name"        // Sets the name for the given tid. The name is provided in a name argument.
	metadataThreadSortIndex  = "thread_sort_index"  // Sets the thread sort order position. The sort index is provided in a sort_index argument.
)

type scope string

const (
	scopeThread  = "t"
	scopeProcess = "p"
	scopeGlobal  = "g"
)

func (e event) MarshalJSON() ([]byte, error) {
	type Alias event

	var shadow struct {
		Alias
		Categories string `json:"cat,omitzero"`
	}

	shadow.Alias = Alias(e)
	if len(e.Categories) != 0 {
		var b strings.Builder
		b.WriteString(string(e.Categories[0]))
		for i := 1; i < len(e.Categories); i++ {
			b.WriteString(fmt.Sprintf(",%s", e.Categories[i]))
		}
		shadow.Categories = b.String()
	}

	return json.Marshal(shadow)
}
