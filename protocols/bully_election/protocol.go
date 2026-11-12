package main

import (
	"fmt"
	"net/rpc"
	"sync"
	"time"
)

// call performs a peer RPC with a timeout.
func call(client *rpc.Client, method string, args, reply any) error {
	done := make(chan error, 1)
	go func() { done <- client.Call(method, args, reply) }()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		return fmt.Errorf("%s: no reply within 3s; remote outcome unknown", method)
	}
}

type Empty struct{}

type ElectionMsg struct {
	From string `json:"from"`
	ID   int    `json:"id"`
	Term uint64 `json:"term"`
}
type ElectionReply struct {
	OK bool `json:"ok"` // true if the receiver outranks the sender
}

type CoordinatorMsg struct {
	From string `json:"from"`
	ID   int    `json:"id"`
	Term uint64 `json:"term"`
}
type CoordinatorReply struct{}

type State struct {
	Node            string `json:"node"`
	ID              int    `json:"id"`
	Role            string `json:"role"` // follower | candidate | coordinator
	Coordinator     string `json:"coordinator"`
	Term            uint64 `json:"term"`
	CoordinatorTerm uint64 `json:"coordinatorTerm"`
	ElectionRound   uint64 `json:"electionRound"`
	ElectionsWon    int    `json:"electionsWon"`
	BackedDown      int    `json:"backedDown"`
	Sent            uint64 `json:"sent"` // election messages sent (Election + Coordinator)
	LastError       string `json:"lastError"`
}

func (s *ElectionService) isMember(id string) bool {
	for _, member := range s.members {
		if member == id {
			return true
		}
	}
	return false
}

func (s *ElectionService) stateLocked() State {
	return State{
		Node: s.id, ID: s.priority, Role: s.role, Coordinator: s.coordinator,
		Term: s.term, CoordinatorTerm: s.coordinatorTerm,
		ElectionRound: s.electionRound, ElectionsWon: s.won, BackedDown: s.backedDown,
		Sent: s.sent, LastError: s.lastErr,
	}
}

func (s *ElectionService) bump() {
	s.mu.Lock()
	s.sent++
	s.mu.Unlock()
}

// sendElection delivers one Election and returns the OK answer.
func (s *ElectionService) sendElection(to string, msg ElectionMsg) (ElectionReply, error) {
	client := s.peers[to]
	if client == nil {
		return ElectionReply{}, fmt.Errorf("no connection to %s", to)
	}
	s.bump()
	var reply ElectionReply
	err := call(client, "ElectionService.Election", msg, &reply)
	return reply, err
}

// sendCoordinator broadcasts the winner announcement.
func (s *ElectionService) sendCoordinator(to string, msg CoordinatorMsg) {
	client := s.peers[to]
	if client == nil {
		return
	}
	s.bump()
	_ = call(client, "ElectionService.Coordinator", msg, &CoordinatorReply{})
}

// startElection begins one round from this node if none is already running.
func (s *ElectionService) startElection() {
	s.mu.Lock()
	if s.role == roleCandidate {
		s.mu.Unlock()
		return
	}
	s.electionRound++
	s.term++
	round := s.electionRound
	term := s.term
	s.role = roleCandidate
	st := s.stateLocked()
	s.mu.Unlock()
	_ = s.report(st)
	s.campaign(round, term)
}

// campaign runs one full round: ask every higher node, then back down or win.
func (s *ElectionService) campaign(round, term uint64) {
	s.mu.Lock()
	self := s.id
	message := ElectionMsg{From: s.id, ID: s.priority, Term: term}
	higher := make([]string, 0, len(s.members))
	for _, member := range s.members {
		if priority(member) > s.priority {
			higher = append(higher, member)
		}
	}
	s.mu.Unlock()

	var workers sync.WaitGroup
	var countMu sync.Mutex
	okCount := 0
	for _, peer := range higher {
		workers.Add(1)
		go func(to string) {
			defer workers.Done()
			reply, err := s.sendElection(to, message)
			if err != nil {
				return // treated as "down"
			}
			if reply.OK {
				countMu.Lock()
				okCount++
				countMu.Unlock()
			}
		}(peer)
	}
	workers.Wait()

	s.mu.Lock()
	if s.electionRound != round || s.role != roleCandidate || s.term != term {
		s.mu.Unlock()
		return // a newer term or Coordinator arrived; we stood down
	}
	if okCount > 0 {
		s.role = roleFollower
		s.backedDown++
		st := s.stateLocked()
		s.mu.Unlock()
		_ = s.report(st)
		return
	}

	// Nobody higher answered: we are the highest node alive.
	s.role = roleCoordinator
	s.coordinator = s.id
	s.coordinatorTerm = term
	s.won++
	victory := CoordinatorMsg{From: s.id, ID: s.priority, Term: term}
	peers := append([]string(nil), s.members...)
	st := s.stateLocked()
	s.mu.Unlock()
	_ = s.report(st)

	var deliver sync.WaitGroup
	for _, peer := range peers {
		if peer == self {
			continue
		}
		deliver.Add(1)
		go func(to string) {
			defer deliver.Done()
			s.sendCoordinator(to, victory)
		}(peer)
	}
	deliver.Wait()
}

// Election replies OK and starts the receiver's own election.
func (s *ElectionService) Election(args ElectionMsg, reply *ElectionReply) error {
	s.mu.Lock()
	if args.From == "" || args.From == s.id || !s.isMember(args.From) {
		s.mu.Unlock()
		return fmt.Errorf("election from unknown node %q", args.From)
	}
	priorTerm := s.term
	if args.Term < s.term {
		// Stale election from an older term: tell the sender to stand down.
		s.mu.Unlock()
		reply.OK = true
		return nil
	}
	if args.Term > s.term {
		s.term = args.Term
	}
	if s.priority <= args.ID {
		s.mu.Unlock()
		reply.OK = false
		return nil
	}
	reply.OK = true

	if s.role == roleCoordinator {
		if args.Term > s.coordinatorTerm {
			s.coordinatorTerm = args.Term
		}
		coord, term := s.coordinator, s.coordinatorTerm
		s.mu.Unlock()
		if coord != "" && coord != args.From {
			go s.sendCoordinator(args.From, CoordinatorMsg{From: coord, ID: priority(coord), Term: term})
		}
		return nil
	}

	if s.role == roleCandidate {
		if s.term > priorTerm {
			// A newer term arrived. Restart above it instead of leaving the
			// old campaign to abort with no replacement.
			s.electionRound++
			s.term++
			round := s.electionRound
			term := s.term
			st := s.stateLocked()
			s.mu.Unlock()
			_ = s.report(st)
			go s.campaign(round, term)
		} else {
			s.mu.Unlock()
		}
		return nil
	}

	s.electionRound++
	round := s.electionRound
	term := s.term
	s.role = roleCandidate
	st := s.stateLocked()
	s.mu.Unlock()
	_ = s.report(st)
	go s.campaign(round, term)
	return nil
}

// Coordinator records the announced coordinator.
func (s *ElectionService) Coordinator(args CoordinatorMsg, reply *CoordinatorReply) error {
	s.mu.Lock()
	if args.From == "" || args.From == s.id || !s.isMember(args.From) {
		s.mu.Unlock()
		return fmt.Errorf("coordinator from unknown node %q", args.From)
	}
	incoming := priority(args.From)
	oldTerm := s.term

	stale := args.Term < s.term
	if !stale && s.coordinator != "" {
		if args.Term < s.coordinatorTerm ||
			(args.Term == s.coordinatorTerm && incoming < priority(s.coordinator)) {
			stale = true
		}
	}
	if stale {
		current, term := s.coordinator, s.coordinatorTerm
		s.mu.Unlock()
		if current != "" && current != args.From {
			go s.sendCoordinator(args.From, CoordinatorMsg{From: current, ID: priority(current), Term: term})
		}
		return nil
	}
	if args.Term > s.term {
		s.term = args.Term
	}

	if incoming <= s.priority {
		role, self := s.role, s.id
		if role == roleCoordinator {
			changed := s.term > s.coordinatorTerm
			if changed {
				s.coordinatorTerm = s.term
			}
			term := s.coordinatorTerm
			st := s.stateLocked()
			s.mu.Unlock()
			if changed {
				_ = s.report(st)
			}
			go s.sendCoordinator(args.From, CoordinatorMsg{From: self, ID: s.priority, Term: term})
			return nil
		}
		if role == roleCandidate {
			// A lower node cannot take over from us. Restart above its term.
			s.electionRound++
			s.term++
			round := s.electionRound
			term := s.term
			st := s.stateLocked()
			s.mu.Unlock()
			_ = s.report(st)
			go s.campaign(round, term)
			return nil
		}
		s.mu.Unlock()
		go s.startElection()
		return nil
	}

	changed := s.coordinator != args.From || s.role != roleFollower ||
		s.coordinatorTerm != args.Term || oldTerm != s.term
	s.coordinator = args.From
	s.coordinatorTerm = args.Term
	if s.role == roleCandidate {
		s.electionRound++ // kills the campaign still in flight
		s.backedDown++
	}
	s.role = roleFollower
	st := s.stateLocked()
	s.mu.Unlock()
	if changed {
		_ = s.report(st)
	}
	return nil
}

// StartElection runs one round from this node.
func (a *Application) StartElection(_ Empty, reply *State) error {
	s := a.service
	s.startElection()
	s.mu.Lock()
	defer s.mu.Unlock()
	*reply = s.stateLocked()
	_ = s.report(*reply)
	return nil
}

func (a *Application) Read(_ Empty, reply *State) error {
	s := a.service
	s.mu.Lock()
	defer s.mu.Unlock()
	*reply = s.stateLocked()
	return s.report(*reply)
}
