package radix

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/mediocregopher/radix/v4/internal/proc"
	"github.com/mediocregopher/radix/v4/resp"
	"github.com/mediocregopher/radix/v4/resp/resp2"
)

var errPubSubMode = resp2.Error{
	E: errors.New("ERR only (P)SUBSCRIBE / (P)UNSUBSCRIBE / PING / QUIT allowed in this context"),
}

type multiMarshal []resp.Marshaler

func (mm multiMarshal) MarshalRESP(w io.Writer) error {
	for _, m := range mm {
		if err := m.MarshalRESP(w); err != nil {
			return err
		}
	}
	return nil
}

type pubSubStub struct {
	proc *proc.Proc
	Conn
	fn   func([]string) interface{}
	inCh <-chan PubSubMessage

	pubsubMode      bool
	subbed, psubbed map[string]bool

	// this is only used for tests
	mDoneCh chan struct{}
}

// PubSubStub returns a (fake) Conn, much like Stub does, which pretends it is a
// Conn to a real redis instance, but is instead using the given callback to
// service requests. It is primarily useful for writing tests.
//
// PubSubStub differes from Stub in that Encode calls for (P)SUBSCRIBE,
// (P)UNSUBSCRIBE, MESSAGE, and PING will be intercepted and handled as per
// redis' expected pubsub functionality. A PubSubMessage may be written to the
// returned channel at any time, and if the PubSubStub has had (P)SUBSCRIBE
// called matching that PubSubMessage it will be written to the PubSubStub's
// internal buffer as expected.
//
// This is intended to be used so that it can mock services which can perform
// both normal redis commands and pubsub (e.g. a real redis instance, redis
// sentinel). Once created this stub can be passed into PubSub and treated like
// a real connection.
func PubSubStub(remoteNetwork, remoteAddr string, fn func([]string) interface{}) (Conn, chan<- PubSubMessage) {
	ch := make(chan PubSubMessage)
	s := &pubSubStub{
		proc:    proc.New(),
		fn:      fn,
		inCh:    ch,
		subbed:  map[string]bool{},
		psubbed: map[string]bool{},
		mDoneCh: make(chan struct{}, 1),
	}
	s.Conn = Stub(remoteNetwork, remoteAddr, s.innerFn)
	s.proc.Run(s.spin)
	return s, ch
}

func (s *pubSubStub) innerFn(ss []string) interface{} {
	var res interface{}
	err := s.proc.WithLock(func() error {
		writeRes := func(mm multiMarshal, cmd, subj string) multiMarshal {
			c := len(s.subbed) + len(s.psubbed)
			s.pubsubMode = c > 0
			return append(mm, resp2.Any{I: []interface{}{cmd, subj, c}})
		}

		switch strings.ToUpper(ss[0]) {
		case "PING":
			if !s.pubsubMode {
				res = s.fn(ss)
			} else {
				res = []string{"pong", ""}
			}
		case "SUBSCRIBE":
			var mm multiMarshal
			for _, channel := range ss[1:] {
				s.subbed[channel] = true
				mm = writeRes(mm, "subscribe", channel)
			}
			res = mm
		case "UNSUBSCRIBE":
			var mm multiMarshal
			for _, channel := range ss[1:] {
				delete(s.subbed, channel)
				mm = writeRes(mm, "unsubscribe", channel)
			}
			res = mm
		case "PSUBSCRIBE":
			var mm multiMarshal
			for _, pattern := range ss[1:] {
				s.psubbed[pattern] = true
				mm = writeRes(mm, "psubscribe", pattern)
			}
			res = mm
		case "PUNSUBSCRIBE":
			var mm multiMarshal
			for _, pattern := range ss[1:] {
				delete(s.psubbed, pattern)
				mm = writeRes(mm, "punsubscribe", pattern)
			}
			res = mm
		case "MESSAGE":
			m := PubSubMessage{
				Type:    "message",
				Channel: ss[1],
				Message: []byte(ss[2]),
			}

			var mm multiMarshal
			if s.subbed[m.Channel] {
				mm = append(mm, m)
			}
			res = mm
		case "PMESSAGE":
			m := PubSubMessage{
				Type:    "pmessage",
				Pattern: ss[1],
				Channel: ss[2],
				Message: []byte(ss[3]),
			}

			var mm multiMarshal
			if s.psubbed[m.Pattern] {
				mm = append(mm, m)
			}
			res = mm
		default:
			if s.pubsubMode {
				return errPubSubMode
			}
			res = s.fn(ss)
		}
		return nil
	})
	if err != nil {
		return err
	}
	return res
}

func (s *pubSubStub) Close() error {
	return s.proc.Close(func() error {
		return s.Conn.Close()
	})
}

func (s *pubSubStub) spin(ctx context.Context) {
	for {
		select {
		case m, ok := <-s.inCh:
			if !ok {
				panic("PubSubStub message channel was closed")
			}
			if m.Type == "" {
				if m.Pattern == "" {
					m.Type = "message"
				} else {
					m.Type = "pmessage"
				}
			}
			if err := s.Conn.EncodeDecode(ctx, m, nil); err != nil {
				panic(fmt.Sprintf("error encoding message in PubSubStub: %s", err))
			}
			select {
			case s.mDoneCh <- struct{}{}:
			default:
			}
		case <-ctx.Done():
			return
		}
	}
}
