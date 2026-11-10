package main

import (
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"sort"
	"time"
)

// Each peer sees exactly one Request RPC [remote procedure call]. Its response
// IS the permission: the handler does not return successfully before granting it.
// There is no separate Ack RPC and no Release RPC.
//
// s.mu protects local state, not the critical section [the work only one node
// may execute at a time]. Never hold s.mu while waiting for another node.
// Methods ending in Locked require the caller to hold s.mu.

type Empty struct{}

// RequestID is fixed for one acquisition. The live logical clock may advance
// while this request waits, but its priority must not change.
type RequestID struct {
	TS   uint64 `json:"ts"`
	Node string `json:"node"`
}

type ReqMsg struct {
	From string `json:"from"`
	TS   uint64 `json:"ts"`
}

// Permission is the Request RPC's response, not another RPC request.
// TS dates the reply; ForTS identifies the original acquisition.
type Permission struct {
	From  string `json:"from"`
	TS    uint64 `json:"ts"`
	ForTS uint64 `json:"forTS"`
}

type AcquireArgs struct {
	HoldMs float64 `json:"holdMs"` // 0 defaults to 2000 ms
}

type AutoArgs struct {
	Count int `json:"count"` // 0 defaults to 5 cycles; maximum 100
}

// State is a snapshot [a copy of local state] for the application and UI.
// AcksHave counts permission responses, not separate acknowledgment calls.
type State struct {
	Node             string   `json:"node"`
	Algorithm        string   `json:"algorithm"`
	ReportSeq        uint64   `json:"reportSeq"`
	Clock            uint64   `json:"clock"`
	Role             string   `json:"role"`
	MyTS             uint64   `json:"myTS"`
	AcksHave         int      `json:"acksHave"`
	AcksNeed         int      `json:"acksNeed"`
	MissingAcks      []string `json:"missingAcks"`
	Deferred         []string `json:"deferred"`
	Entries          int      `json:"entries"`
	Completed        int      `json:"completed"`
	LastWaitMs       float64  `json:"lastWaitMs"`
	MessagesSent     uint64   `json:"messagesSent"`
	SendErrors       uint64   `json:"sendErrors"`
	LastError        string   `json:"lastError"`
	Operation        string   `json:"operation"`
	OperationRunning bool     `json:"operationRunning"`
	AutoRunning      bool     `json:"autoRunning"`
}

// Earlier timestamps win. Numeric node IDs break ties; the string fallback
// keeps the order unique even for names with the same numeric suffix.
func requestLess(a, b RequestID) bool {
	if a.TS != b.TS {
		return a.TS < b.TS
	}
	if priority(a.Node) != priority(b.Node) {
		return priority(a.Node) < priority(b.Node)
	}
	return a.Node < b.Node
}

func (s *MutexService) validPeer(id string) bool {
	if id == s.id {
		return false
	}
	for _, member := range s.members {
		if member == id {
			return true
		}
	}
	return false
}

func (s *MutexService) stateLocked() State {
	requests := make([]RequestID, 0, len(s.deferred))
	for request := range s.deferred {
		requests = append(requests, request)
	}
	sort.Slice(requests, func(i, j int) bool { return requestLess(requests[i], requests[j]) })
	deferred := make([]string, 0, len(requests))
	for _, request := range requests {
		deferred = append(deferred, request.Node)
	}
	missing := make([]string, 0)
	if s.role == roleWaiting {
		for _, node := range s.members {
			if node != s.id && !s.acks[node] {
				missing = append(missing, node)
			}
		}
	}
	return State{
		Node: s.id, Algorithm: "ricart-agrawala", ReportSeq: s.reportSeq,
		Clock: s.clock.now, Role: s.role, MyTS: s.myTS,
		AcksHave: len(s.acks), AcksNeed: len(s.members) - 1, MissingAcks: missing,
		Deferred: deferred, Entries: s.entries, Completed: s.completed,
		LastWaitMs: s.lastWaitMS, MessagesSent: s.sent, SendErrors: s.sendErrors,
		LastError: s.lastErr, Operation: s.operation,
		OperationRunning: s.operationRunning, AutoRunning: s.autoRunning,
	}
}

// Publish under the state lock so an old snapshot cannot follow a newer one.
func (s *MutexService) reportLocked() error {
	s.reportSeq++
	err := s.report(s.stateLocked())
	if err != nil {
		log.Printf("%s state report: %v", s.id, err)
	}
	return err
}

func (s *MutexService) errorLocked(err error) error {
	s.lastErr = err.Error()
	_ = s.reportLocked()
	return err
}

func (s *MutexService) blockedByMeLocked(their RequestID) bool {
	return s.role == roleCritical || (s.role == roleWaiting &&
		requestLess(RequestID{TS: s.myTS, Node: s.id}, their))
}

// Request returns permission only when this node is entitled to grant it.
// Waiting handlers release s.mu, allowing local acquisition/release and other
// incoming requests to run. Shutdown returns an error, never permission.
func (s *MutexService) Request(args ReqMsg, reply *Permission) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	if !s.validPeer(args.From) || args.TS == 0 {
		return s.errorLocked(fmt.Errorf("invalid request from %q", args.From))
	}
	s.clock.onReceive(args.TS)
	request := RequestID{TS: args.TS, Node: args.From}
	if s.blockedByMeLocked(request) {
		// Capture this acquisition's channel while inspecting its state. A new
		// acquisition may replace s.released before this handler wakes up.
		released := s.released
		s.deferred[request]++
		_ = s.reportLocked()
		s.mu.Unlock()
		select {
		case <-released:
		case <-s.ctx.Done():
		}
		s.mu.Lock()
		s.deferred[request]--
		if s.deferred[request] == 0 {
			delete(s.deferred, request)
		}
		if s.ctx.Err() != nil {
			_ = s.reportLocked()
			return s.ctx.Err()
		}
		// Do not recheck against a NEW local request: the old acquisition has
		// released, and that newer request must obtain this peer's permission.
	}
	*reply = Permission{From: s.id, TS: s.clock.tick(), ForTS: args.TS}
	// Count the outgoing permission response, not just outgoing RPC requests.
	// This records preparation for sending, not proof of delivery.
	s.sent++
	_ = s.reportLocked()
	return nil
}

func (s *MutexService) tryEnterLocked() {
	if s.ctx.Err() != nil || s.role != roleWaiting || len(s.acks) != len(s.members)-1 {
		return
	}
	s.role, s.operation = roleCritical, "critical"
	s.entries++
	s.lastWaitMS = float64(time.Since(s.waitStart).Microseconds()) / 1000
	_ = s.reportLocked()
	// Report entry before allowing the worker to start its hold period.
	close(s.entered)
	s.entered = nil
}

// acceptPermissionLocked is a LOCAL helper, not a remotely callable Ack method.
// Check both the called peer and the exact request identity before counting it.
func (s *MutexService) acceptPermissionLocked(peer string, request ReqMsg, reply Permission) error {
	if !s.validPeer(peer) || reply.From != peer || reply.TS == 0 || reply.ForTS != request.TS {
		return fmt.Errorf("invalid permission response from %q for request %d", peer, request.TS)
	}
	s.clock.onReceive(reply.TS)
	if s.ctx.Err() == nil && s.role == roleWaiting && s.myTS == request.TS {
		s.acks[peer] = true
		s.tryEnterLocked()
	}
	_ = s.reportLocked()
	return nil
}

// One RPC per peer. Its completion now MEANS permission, rather than an early
// empty receipt. There is no added timer or retry. The unchanged SDK still
// imposes its ten-second deadline; a timeout is an error, never permission.
func (s *MutexService) requestPermission(peer string, request ReqMsg) {
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		return
	}
	s.sent++
	_ = s.reportLocked()
	client := s.peers[peer]
	s.mu.Unlock()

	var reply Permission
	var err error
	if client == nil {
		err = fmt.Errorf("no client for peer")
	} else {
		call := client.Go("MutexService.Request", request, &reply, nil)
		select {
		case result := <-call.Done:
			err = result.Error
		case <-s.ctx.Done():
			// Do not read reply here: the RPC client may still be writing it.
			return
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return
	}
	if err == nil {
		err = s.acceptPermissionLocked(peer, request, reply)
	}
	if err != nil {
		s.sendErrors++
		_ = s.errorLocked(fmt.Errorf("Request to %s: %w", peer, err))
		// Failure is not permission. Stay waiting; there is no retry or
		// abandonment protocol. MissingAcks shows which peer still blocks us.
	}
}

// Reserve an acquisition before returning from the application method. A
// second local caller is rejected rather than changing a live request's identity.
func (s *MutexService) beginLocked() (<-chan struct{}, ReqMsg, error) {
	if s.ctx.Err() != nil {
		return nil, ReqMsg{}, s.ctx.Err()
	}
	if s.role != roleIdle || s.operationRunning {
		return nil, ReqMsg{}, fmt.Errorf("an acquisition is already %s", s.operation)
	}
	s.myTS = s.clock.tick()
	s.role, s.operation = roleWaiting, "waiting"
	s.operationRunning = true
	s.lastErr = ""
	s.acks = make(map[string]bool)
	s.released = make(chan struct{})
	s.waitStart = time.Now()
	s.entered = make(chan struct{})
	entered := s.entered
	_ = s.reportLocked()
	s.tryEnterLocked() // One node needs no peer permissions.
	return entered, ReqMsg{From: s.id, TS: s.myTS}, nil
}

func (s *MutexService) runCycle(entered <-chan struct{}, request ReqMsg, hold time.Duration) bool {
	for _, peer := range s.members {
		if peer != s.id {
			go s.requestPermission(peer, request)
		}
	}
	select {
	case <-entered:
	case <-s.ctx.Done():
		s.stopped()
		return false
	}
	timer := time.NewTimer(hold)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-s.ctx.Done():
		s.stopped()
		return false
	}
	s.release()
	return true
}

func (s *MutexService) stopped() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operationRunning = false
	s.operation = "stopped"
	// Shutdown is not successful completion; retain the last role for inspection.
	_ = s.reportLocked()
}

func (s *MutexService) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.role, s.operation = roleIdle, "completed"
	s.myTS = 0
	s.acks = nil
	s.completed++
	s.operationRunning = false
	// Record that we no longer hold the section BEFORE waking peer handlers.
	_ = s.reportLocked()
	close(s.released)
	s.released = nil
}

func holdDuration(ms float64) (time.Duration, error) {
	if ms == 0 {
		ms = 2000
	}
	if math.IsNaN(ms) || math.IsInf(ms, 0) || ms < 1 || ms > 120000 {
		return 0, fmt.Errorf("holdMs must be between 1 and 120000 ms (0 selects 2000)")
	}
	return time.Duration(ms * float64(time.Millisecond)), nil
}

// Acquire accepts a background operation; success here does not mean acquired.
// Observe role/entries/completed for progress. This application response is not
// an inter-node permission and does not count toward the peer message total.
func (a *Application) Acquire(args AcquireArgs, reply *State) error {
	s := a.service
	s.mu.Lock()
	defer s.mu.Unlock()
	hold, err := holdDuration(args.HoldMs)
	if err != nil {
		return s.errorLocked(err)
	}
	if s.autoRunning {
		return s.errorLocked(fmt.Errorf("auto mode owns the acquisition sequence"))
	}
	entered, request, err := s.beginLocked()
	if err != nil {
		return s.errorLocked(err)
	}
	*reply = s.stateLocked()
	go s.runCycle(entered, request, hold)
	return nil
}

// Auto runs a bounded number of local cycles, one at a time.
func (a *Application) Auto(args AutoArgs, reply *State) error {
	s := a.service
	s.mu.Lock()
	defer s.mu.Unlock()
	count := args.Count
	if count == 0 {
		count = 5
	}
	if count < 1 || count > 100 {
		return s.errorLocked(fmt.Errorf("count must be between 1 and 100 (0 selects 5)"))
	}
	if s.autoRunning || s.operationRunning {
		return s.errorLocked(fmt.Errorf("an acquisition or auto sequence is already running"))
	}
	s.autoRunning = true
	entered, request, err := s.beginLocked()
	if err != nil {
		s.autoRunning = false
		return s.errorLocked(err)
	}
	*reply = s.stateLocked()
	go func() {
		defer func() {
			s.mu.Lock()
			s.autoRunning = false
			_ = s.reportLocked()
			s.mu.Unlock()
		}()
		for i := 0; i < count; i++ {
			if !s.runCycle(entered, request, time.Second) || i == count-1 {
				return
			}
			timer := time.NewTimer(time.Duration(100+rand.IntN(500)) * time.Millisecond)
			select {
			case <-timer.C:
			case <-s.ctx.Done():
				timer.Stop()
				return
			}
			s.mu.Lock()
			entered, request, err = s.beginLocked()
			if err != nil {
				_ = s.errorLocked(err)
			}
			s.mu.Unlock()
			if err != nil {
				return
			}
		}
	}()
	return nil
}

func (a *Application) Read(_ Empty, reply *State) error {
	s := a.service
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reportLocked(); err != nil {
		return err
	}
	*reply = s.stateLocked()
	return nil
}
