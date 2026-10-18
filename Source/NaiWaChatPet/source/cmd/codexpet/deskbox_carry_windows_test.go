package main

import (
	"os"
	"testing"
	"time"
)

func TestDeskBoxCarryWaitsForReceiptAndNeverMovesFiles(t *testing.T) {
	a, m := carryFixture(t)
	a.trip.session = nil
	a.trip.external = &externalCarry{id: "carry-test", ready: make(chan error, 1)}
	a.pickUp(m, 0, 0)
	a.trip.stage = tripToFolder
	a.tripArrived(time.Second)
	if a.trip.stage != tripAwaitReceipt || !a.trip.external.arrived {
		t.Fatal("did not wait for DeskBox receipt")
	}
	if err := <-a.trip.external.ready; err != nil {
		t.Fatal(err)
	}
	if a.trip.delivered != 0 {
		t.Fatal("reported success before commit")
	}
	// A successful receipt only authorizes the drop animation; it grants no Go file write.
	a.trip.external.success = true
	a.deliver(2 * time.Second)
	if a.trip.delivered != 1 || len(a.play.carrying) != 0 {
		t.Fatal("drop animation state")
	}
	if _, err := os.Stat(m.From); err != nil {
		t.Fatal("Go moved the source", err)
	}
	if _, err := os.Stat(m.To); !os.IsNotExist(err) {
		t.Fatal("Go created destination")
	}
}

func TestDeskBoxCarryFailedReceiptDoesNotReportDelivery(t *testing.T) {
	a, m := carryFixture(t)
	a.trip.session = nil
	a.trip.external = &externalCarry{success: false}
	a.pickUp(m, 0, 0)
	a.deliver(time.Second)
	if a.trip.delivered != 0 {
		t.Fatal("failed file reported delivered")
	}
	if _, err := os.Stat(m.From); err != nil {
		t.Fatal("original missing", err)
	}
}
