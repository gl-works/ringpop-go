// Copyright (c) 2015 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package swim

import (
	"sync"
	"time"

	log "github.com/uber-common/bark"
	"github.com/gl-works/ringpop-go/logging"
)

type suspect interface {
	address() string
	incarnation() int64
}

// Suspicion handles the suspicion sub-protocol of the SWIM protocol
type suspicion struct {
	sync.Mutex

	node *Node

	timeout time.Duration
	timers  map[string]*time.Timer
	enabled bool
	logger  log.Logger
}

// newSuspicion returns a new suspicion SWIM sub-protocol with the given timeout
func newSuspicion(n *Node, timeout time.Duration) *suspicion {
	suspicion := &suspicion{
		node:    n,
		timeout: timeout,
		timers:  make(map[string]*time.Timer),
		enabled: true,
		logger:  logging.Logger("suspicion").WithField("local", n.Address()),
	}

	return suspicion
}

func (s *suspicion) Start(suspect suspect) {
	s.withLock(func() {
		if !s.enabled {
			s.logger.Warn("cannot start suspect period while disabled")
			return
		}

		if s.node.Address() == suspect.address() {
			s.logger.Warn("cannot start suspect period for local member")
			return
		}

		if _, ok := s.timers[suspect.address()]; ok {
			s.logger.Warn("redundant call to start suspect ignored")
			return
		}

		s.timers[suspect.address()] = time.AfterFunc(s.timeout, func() {
			s.logger.WithField("faulty", suspect.address()).Info("member declared faulty")
			s.node.memberlist.MakeFaulty(suspect.address(), suspect.incarnation())
		})

		s.logger.WithField("suspect", suspect.address()).Debug("started member suspect period")
	})
}

func (s *suspicion) Stop(suspect suspect) {
	s.Lock()

	if timer, ok := s.timers[suspect.address()]; ok {
		timer.Stop()
		delete(s.timers, suspect.address())
		s.logger.WithField("suspect", suspect.address()).Debug("stopped member suspect period")
	}

	s.Unlock()
}

// reenable suspicion protocol
func (s *suspicion) Reenable() {
	s.Lock()

	if s.enabled {
		s.logger.Warn("suspicion already enabled")
		s.Unlock()
		return
	}

	s.enabled = true
	s.Unlock()
	s.logger.Info("reenabled suspicion protocol")
}

// stop all suspicion timers and disables suspicion protocol
func (s *suspicion) Disable() {
	s.Lock()

	if !s.enabled {
		s.logger.Warn("suspicion already disabled")
		s.Unlock()
		return
	}

	s.enabled = false

	numTimers := len(s.timers)
	for address, timer := range s.timers {
		timer.Stop()
		delete(s.timers, address)
	}

	s.Unlock()
	s.logger.WithField("timersStopped", numTimers).Info("disabled suspicion protocol")
}

// testing func to avoid data races
func (s *suspicion) Timer(address string) *time.Timer {
	var rv *time.Timer
	s.withLock(func() {
		rv = s.timers[address]
	})
	return rv
}

func (s *suspicion) withLock(f func()) {
	s.Lock()
	f()
	s.Unlock()
}
