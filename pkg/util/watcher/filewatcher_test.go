package watcher

import (
	"testing"

	"github.com/rjeczalik/notify"
	"golang.org/x/sys/unix"
)

type testEventWrapper struct {
	event notify.Event
}

func (t *testEventWrapper) Event() notify.Event {
	return t.event
}

func (t *testEventWrapper) Path() string     { return "" }
func (t *testEventWrapper) Sys() interface{} { return &unix.InotifyEvent{} }

var _ notify.EventInfo = &testEventWrapper{}

func testEvent(event notify.Event) notify.EventInfo {
	return &testEventWrapper{event}
}

var testEvents = []notifyEvents{
	{
		testEvent(notify.InDelete),
		testEvent(notify.InCloseWrite),
		testEvent(notify.InMovedTo),
	},
	{
		testEvent(notify.InCloseWrite),
		testEvent(notify.InDelete),
		testEvent(notify.InDelete),
	},
	{
		testEvent(notify.InCloseWrite),
		testEvent(notify.InMovedTo),
		testEvent(notify.InMovedFrom),
		testEvent(notify.InDelete),
	},
	{
		testEvent(notify.InDelete),
		testEvent(notify.InCloseWrite),
	},
	{
		testEvent(notify.InCloseWrite),
		testEvent(notify.InDelete),
	},
	{
		testEvent(notify.InMovedFrom),
		testEvent(notify.InCloseWrite),
	},
}

var targets = []FileEvents{
	{
		FileEventModify,
	},
	{
		FileEventDelete,
	},
	{
		FileEventModify,
		FileEventMove,
		FileEventDelete,
	},
	{
		FileEventModify,
	},
	{},
	{
		FileEventModify,
	},
}

func extractEvents(updates FileUpdates) (events FileEvents) {
	for _, update := range updates {
		events = append(events, update.Event)
	}

	return
}

func eventsEqual(a, b FileEvents) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestEventConcatenation(t *testing.T) {
	for i, e := range testEvents {
		result := extractEvents((&FileWatcher{}).concatenateEvents(e))
		if !eventsEqual(result, targets[i]) {
			t.Errorf("wrong concatenation result: %v != %v", result, targets[i])
		}
	}
}
