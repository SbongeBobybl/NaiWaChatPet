// The fixture is built only by integration tests; it is not a pet release asset.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"sync"

	ai "codexpet/reconstruction/aiorganization"
)

type fake struct{}

func (fake) Configured() bool { return true }
func (fake) Classify(ctx context.Context, r ai.Request) (ai.Result, error) {
	items := []ai.Suggestion{}
	for _, i := range r.Items {
		items = append(items, ai.Suggestion{ItemID: i.ItemID, CategoryID: r.Categories[0].ID, Confidence: 0.8, Reason: "isolated fixture"})
	}
	return ai.Result{Items: items, Intents: []ai.Intent{}}, nil
}
func (fake) ParseIntent(ctx context.Context, r ai.Request) (ai.Result, error) {
	return ai.Result{Items: []ai.Suggestion{}, Intents: []ai.Intent{{CategoryID: r.Categories[0].ID, Position: "top_left", Collapsed: true}}}, nil
}

// carryState mirrors the shipped pet contract: a walk only reports arrival, and a
// receipt must name the single carry currently in flight. The fixture never
// touches files, so a file move can only be observed through these receipts.
type carryState struct {
	mu        sync.Mutex
	id        string
	completed bool
	receipts  int
	refused   int
}

var state = &carryState{}

func (fake) Carry(ctx context.Context, r ai.Request) error {
	state.mu.Lock()
	state.id, state.completed = r.RequestID, false
	state.mu.Unlock()
	switch r.Carry.DestinationName {
	case "refuse-target":
		return errors.New("destination unavailable")
	case "hold-until-cancelled":
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (fake) CompleteCarry(ctx context.Context, r ai.Request) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if r.CarryRequestID != state.id || state.completed {
		return errors.New("stale carry receipt")
	}
	if r.Succeeded {
		state.completed = true
	} else {
		state.refused++
	}
	state.receipts++
	return nil
}

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	s, err := ai.Start(os.Args[1], fake{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer s.Close()
	fmt.Println("ready")
	bufio.NewReader(os.Stdin).ReadString('\n')
}
