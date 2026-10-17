package webaccess

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSlowSubscriberIsBoundedAndDoesNotBlockFastSubscriber(t *testing.T) {
	b := NewBroker()
	slow := &subscriber{frames: make(chan []byte, maxQueueFrames), done: make(chan struct{})}
	fast := &subscriber{frames: make(chan []byte, maxQueueFrames), done: make(chan struct{})}
	b.clients[slow] = true
	b.clients[fast] = true
	for i := 1; i <= maxQueueFrames+10; i++ {
		b.Publish(Event{Name: "session:data", Data: strings.Repeat("x", 16<<10), Sequence: uint64(i)})
		data := <-fast.frames
		b.mu.Lock()
		fast.bytes -= len(data)
		b.mu.Unlock()
		var event Event
		if err := json.Unmarshal(data, &event); err != nil {
			t.Fatal(err)
		}
		if event.Sequence != uint64(i) {
			t.Fatal(event.Sequence)
		}
	}
	select {
	case <-slow.done:
	default:
		t.Fatal("slow subscriber not disconnected")
	}
	if slow.bytes > maxQueueBytes || len(slow.frames) > maxQueueFrames {
		t.Fatal("unbounded queue")
	}
	select {
	case <-fast.done:
		t.Fatal("fast subscriber was disconnected")
	default:
	}
	b.DisconnectAll()
	select {
	case <-fast.done:
	default:
		t.Fatal("disconnect did not end fast stream")
	}
}
