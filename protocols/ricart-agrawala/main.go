// distvis:name Ricart–Agrawala Mutual Exclusion
// distvis:description One request and one deferred permission response per peer. No separate Ack or Release calls.
// distvis:start node-1 Application.Acquire {"holdMs":2000} Start an acquisition; watch operation progress from waiting to critical to completed.
package main

import (
	"context"
	"net/rpc"
	"strconv"
	"strings"
	"sync"
	"time"

	lab "distvis/sdk/rpc"
)

func main() { lab.Main(configure) }

// priority compares numeric node IDs, so node-2 precedes node-10.
func priority(id string) int {
	parts := strings.Split(id, "-")
	n, _ := strconv.Atoi(parts[len(parts)-1])
	return n
}

// The service mutex protects every clock access.
type lamportClock struct{ now uint64 }

func (c *lamportClock) tick() uint64 {
	c.now++
	return c.now
}

func (c *lamportClock) onReceive(remote uint64) uint64 {
	if remote > c.now {
		c.now = remote
	}
	return c.tick()
}

const (
	roleIdle     = "idle"
	roleWaiting  = "waiting"
	roleCritical = "critical"
)

type MutexService struct {
	mu      sync.Mutex
	id      string
	members []string
	peers   map[string]*rpc.Client
	report  func(any) error
	ctx     context.Context
	clock   lamportClock
	role    string
	myTS    uint64
	acks    map[string]bool

	// Each acquisition creates its own channel. Closing it releases the RPC
	// handlers waiting for THAT acquisition, even if a new one has already begun.
	released chan struct{}
	// Count handlers per request identity so duplicate RPCs remain visible until
	// all their handlers finish. There is no distributed request queue in RA.
	deferred map[RequestID]int

	entries          int
	completed        int
	lastWaitMS       float64
	waitStart        time.Time
	entered          chan struct{}
	operationRunning bool
	operation        string
	autoRunning      bool
	sent             uint64
	sendErrors       uint64
	lastErr          string
	reportSeq        uint64
}

type Application struct{ service *MutexService }

func configure(node *lab.Runtime) error {
	// Keep the SDK's ten-second RPC deadline unchanged. Long deferred replies
	// can time out; an error never counts as permission and there is no retry.
	peers, err := node.RPCPeers()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &MutexService{
		id: node.ID, members: node.Nodes, peers: peers, report: node.Report,
		ctx: ctx, role: roleIdle, operation: "none", deferred: make(map[RequestID]int),
	}
	// Inputs can arrive before OnStart runs. Keep the same context throughout.
	node.OnStart(func(ctx context.Context) error {
		<-ctx.Done()
		cancel()
		return nil
	})
	if err := node.RegisterName("MutexService", s); err != nil {
		cancel()
		return err
	}
	if err := node.RegisterName("Application", &Application{service: s}, lab.Application()); err != nil {
		cancel()
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reportLocked()
}
