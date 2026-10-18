//go:build windows

package main

import (
	"bytes"
	"codexpet/reconstruction/animation"
	"codexpet/reconstruction/play"
	"codexpet/reconstruction/rig"
	"codexpet/reconstruction/tidy"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func carryFixture(t *testing.T, counts ...int) (*desktopHost, tidy.Move) {
	t.Helper()
	root := t.TempDir()
	from := filepath.Join(root, "notes.txt")
	if e := os.WriteFile(from, []byte("original"), 0600); e != nil {
		t.Fatal(e)
	}
	past := time.Now().Add(-time.Hour)
	os.Chtimes(from, past, past)
	count := 1
	if len(counts) > 0 {
		count = counts[0]
	}
	for i := 1; i < count; i++ {
		name := filepath.Join(root, fmt.Sprintf("notes%d.txt", i))
		os.WriteFile(name, []byte("original"), 0600)
		os.Chtimes(name, past, past)
	}
	plan, e := tidy.Scan(root, tidy.Options{})
	if e != nil || len(plan.Moves) != count {
		t.Fatalf("scan: %v %v", plan, e)
	}
	session, e := tidy.Begin(plan, t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { session.Finish() })
	session.Prepare(plan.Moves)
	assets, e := loadAssets(bundledAssets, "runtime.json")
	if e != nil {
		t.Fatal(e)
	}
	stand, e := rig.Stand()
	if e != nil {
		t.Fatal(e)
	}
	a := &desktopHost{assets: assets, w: 420, h: 560, epoch: time.Now(), play: &playState{ctl: play.New(play.Config{Unit: 1}), stand: stand, textures: map[string]*rig.Texture{}, carrying: []string{}}}
	a.trip = &tidyTrip{session: session, groups: []tripGroup{{folder: filepath.Dir(plan.Moves[0].To), moves: append([]tidy.Move(nil), plan.Moves...)}}, icons: map[string]deskIcon{}, total: 1}
	return a, plan.Moves[0]
}
func TestCarryMovesOnlyWhenDelivered(t *testing.T) {
	a, m := carryFixture(t)
	a.pickUp(m, 0, 0)
	if _, e := os.Stat(m.From); e != nil {
		t.Fatal("pickup moved file before arrival", e)
	}
	if _, e := os.Stat(m.To); !os.IsNotExist(e) {
		t.Fatal("destination appeared before delivery")
	}
	if len(a.trip.pending) != 1 || len(a.play.carrying) != 1 {
		t.Fatal("box did not contain file")
	}
	a.deliver(time.Second)
	if _, e := os.Stat(m.To); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(m.From); !os.IsNotExist(e) {
		t.Fatal("source not moved")
	}
	if a.trip.delivered != 1 || len(a.trip.pending) != 0 || len(a.play.carrying) != 0 {
		t.Fatal("box/progress not emptied")
	}
	report, e := a.trip.session.Finish()
	if e != nil || len(report.Moved) != 1 {
		t.Fatal(report, e)
	}
}
func TestCarryRechecksFileAtDelivery(t *testing.T) {
	a, m := carryFixture(t)
	a.pickUp(m, 0, 0)
	os.WriteFile(m.From, []byte("changed while pet was walking"), 0600)
	a.deliver(time.Second)
	if _, e := os.Stat(m.From); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(m.To); !os.IsNotExist(e) {
		t.Fatal("changed file moved")
	}
	if a.trip.delivered != 0 || a.trip.failed != 1 {
		t.Fatal("incorrect success count")
	}
}
func TestCarryCancellationBeforeDeliveryKeepsSource(t *testing.T) {
	a, m := carryFixture(t)
	a.pickUp(m, 0, 0)
	// Closing before delivery only closes the journal; it must not commit the box.
	report, e := a.trip.session.Finish()
	if e != nil || len(report.Moved) != 0 {
		t.Fatal(report, e)
	}
	if _, e := os.Stat(m.From); e != nil {
		t.Fatal(e)
	}
}
func TestCarryFrameIncludesBoxWhileStationary(t *testing.T) {
	a, _ := carryFixture(t)
	body, e := a.assets.Bodies.BodyCopy(string(animation.Idle), 0)
	if e != nil {
		t.Fatal(e)
	}
	plain, e := a.play.stand.Render(rig.NewTexture(body), rig.StandPose(a.play.view.Gait))
	if e != nil {
		t.Fatal(e)
	}
	frame, e := a.playFrame(animation.Snapshot{State: animation.Idle, Frame: 0}, body, "")
	if e != nil {
		t.Fatal(e)
	}
	if out := os.Getenv("NAIWA_CARRY_FRAME"); out != "" {
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		err = png.Encode(f, frame)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if frame == nil || bytes.Equal(frame.Pix, plain.Pix) {
		t.Fatal("carry frame did not draw box")
	}
}

func TestCarryEveryFileWalksAndVisitsDestination(t *testing.T) {
	a, _ := carryFixture(t, 6)
	moves := append([]tidy.Move(nil), a.trip.groups[0].moves...)
	for i, m := range moves {
		a.trip.icons[strings.ToLower(m.From)] = deskIcon{path: m.From, x: 100 + i*80, y: 100}
	}
	folder := a.trip.groups[0].folder
	a.trip.icons[strings.ToLower(folder)] = deskIcon{path: folder, x: 600, y: 400}
	for range moves {
		a.nextLeg(0)
		if a.trip.stage != tripToFile {
			t.Fatal("did not walk to each file", a.trip.stage)
		}
		a.tripArrived(time.Second)
		if a.trip.stage != tripPick {
			t.Fatal("did not reach pickup")
		}
		a.pickUp(a.trip.target, time.Second, 0)
		a.nextLeg(2 * time.Second)
		if a.trip.stage != tripToFolder {
			t.Fatal("did not walk to destination", a.trip.stage)
		}
		a.tripArrived(3 * time.Second)
		if a.trip.stage != tripDrop {
			t.Fatal("did not reach drop")
		}
		a.deliver(3 * time.Second)
	}
	if a.trip.delivered != 6 {
		t.Fatal("files after fourth were bypassed")
	}
	a.nextLeg(4 * time.Second)
	if a.trip.stage != tripStow {
		t.Fatal("did not stow box")
	}
}
