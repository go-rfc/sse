package sse_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mubit/sse"
	"github.com/mubit/sse/tests"
	"github.com/stretchr/testify/assert"
)

const (
	eventStream = "text/event-stream"
	textPlain   = "text/plain; charset=utf-8"
)

type server struct {
	ContentType string
	Events      chan []byte
	Hang        bool
	Reconnects  int
	closer      chan struct{}
}

func newServer() (*httptest.Server, *server) {
	config := &server{
		ContentType: eventStream,
		Reconnects:  1,
		Events:      make(chan []byte),
		closer:      make(chan struct{}),
	}
	return httptest.NewServer(config), config
}

func (s *server) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	rw.Header().Set("Connection", "keep-alive")
	rw.Header().Set("Content-Type", s.ContentType)
	if s.Reconnects <= 0 {
		rw.WriteHeader(http.StatusNoContent)
		return
	}
	s.Reconnects--
	f, _ := rw.(http.Flusher)
	f.Flush()

	for {
		select {
		case event, ok := <-s.Events:
			if !ok {
				return
			}
			rw.Write(event)
			f.Flush()
		case <-s.closer:
			return
		}
	}
}

func (s *server) SendAndClose(data []byte) {
	s.Send(data)
	s.Close()
}

func (s *server) Send(data []byte) {
	s.Events <- data
}

func (s *server) Close() {
	s.closer <- struct{}{}
}

func assertCloses(t *testing.T, es sse.EventSource) bool {
	es.Close()
	maxWaits := 10
	var waits int
	for es.ReadyState() == sse.Closing && waits < maxWaits {
		time.Sleep(10 * time.Millisecond)
		waits++
	}
	return assert.Equal(t, sse.Closed, es.ReadyState())
}

func assertIsOpen(t *testing.T, es sse.EventSource, err error) bool {
	return assert.Nil(t, err) && assert.Equal(t, sse.Open, es.ReadyState())
}

func TestNewEventSourceWithInvalidContentType(t *testing.T) {
	s, config := newServer()
	config.ContentType = textPlain
	es, err := sse.NewEventSource(s.URL)
	if assert.Error(t, err) {
		assert.Equal(t, sse.ErrContentType, err)
		assert.Equal(t, s.URL, es.URL())
		assert.Equal(t, sse.Closed, es.ReadyState())
		_, ok := <-es.Events()
		assert.False(t, ok)
	}
	assertCloses(t, es)
}

func TestEventSourceStates(t *testing.T) {
	for _, test := range []struct {
		stateNumber   byte
		expectedState sse.ReadyState
	}{
		{0, sse.Connecting},
		{1, sse.Open},
		{2, sse.Closing},
		{3, sse.Closed},
	} {
		assert.Equal(t, test.expectedState, sse.ReadyState(test.stateNumber))
	}
}

func TestNewEventSourceWithRightContentType(t *testing.T) {
	s, config := newServer()
	es, err := sse.NewEventSource(s.URL)
	if assertIsOpen(t, es, err) {
		ev := tests.NewEventWithPadding(128)
		go config.SendAndClose(ev)
		recv, ok := <-es.Events()
		if assert.True(t, ok) {
			assert.Equal(t, tests.GetPaddedEventData(ev), recv.Data)
		}
	}
	assertCloses(t, es)
}

func TestNewEventSourceSendingEvent(t *testing.T) {
	expectedEvent := tests.NewEventWithPadding(2 << 10)
	s, config := newServer()
	es, err := sse.NewEventSource(s.URL)
	if assertIsOpen(t, es, err) {
		go config.SendAndClose(expectedEvent)
		ev, ok := <-es.Events()
		if assert.True(t, ok) {
			assert.Equal(t, tests.GetPaddedEventData(expectedEvent), ev.Data)
		}
	}
	assertCloses(t, es)
}

func TestEventSourceLastEventID(t *testing.T) {
	ev := tests.NewEventWithPadding(2 << 8)
	expectedData := tests.GetPaddedEventData(ev)
	ev = append([]byte("id: 123\n"), ev...)
	expectedID := "123"

	s, config := newServer()
	es, err := sse.NewEventSource(s.URL)
	if assertIsOpen(t, es, err) {
		go config.Send(ev)
		ev, ok := <-es.Events()
		if assert.True(t, ok) {
			assert.Equal(t, expectedID, es.LastEventID())
			assert.Equal(t, expectedData, ev.Data)
		}

		go config.Send(tests.NewEventWithPadding(32))
		_, ok = <-es.Events()
		if assert.True(t, ok) {
			assert.Equal(t, expectedID, es.LastEventID())
		}
	}
	assertCloses(t, es)
}

func TestEventSourceRetryIsRespected(t *testing.T) {
	s, config := newServer()
	config.Reconnects = 3
	es, err := sse.NewEventSource(s.URL)
	if assertIsOpen(t, es, err) {
		// Big retry
		config.Send([]byte("retry: 100\n"))
		config.Close()
		go config.Send(tests.NewEventWithPadding(128))
		select {
		case _, ok := <-es.Events():
			assert.True(t, ok)
		case <-timeout(150 * time.Millisecond):
			assert.Fail(t, "event source did not reconnect within the allowed time.")
		}

		// Smaller retry
		config.Send([]byte("retry: 1\n"))
		config.Close()
		go config.Send(tests.NewEventWithPadding(128))
		select {
		case _, ok := <-es.Events():
			assert.True(t, ok)
		case <-timeout(10 * time.Millisecond):
			assert.Fail(t, "event source did not reconnect within the allowed time.")
		}
	}
}

func TestDropConnectionCannotReconnect(t *testing.T) {
	s, config := newServer()
	es, err := sse.NewEventSource(s.URL)
	if assertIsOpen(t, es, err) {
		config.Close()
		go config.Send(tests.NewEventWithPadding(128))
		_, ok := <-es.Events()
		if assert.False(t, ok) {
			assert.Equal(t, sse.Closed, es.ReadyState())
		}
	}
}

func TestDropConnectionCanReconnect(t *testing.T) {
	s, config := newServer()
	config.Reconnects = 2

	es, err := sse.NewEventSource(s.URL)
	if assertIsOpen(t, es, err) {
		config.Close()
		go config.Send(tests.NewEventWithPadding(128))
		_, ok := <-es.Events()
		if assert.True(t, ok) {
			assert.Equal(t, sse.Open, es.ReadyState())
		}
	}
}

func timeout(d time.Duration) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		time.Sleep(d)
		ch <- struct{}{}
	}()
	return ch
}
