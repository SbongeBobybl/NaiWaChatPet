package animation

import (
 "math"
 "testing"
 "time"
)

func testConfig() Config {
 clips:=map[State]Clip{}
 for _,s:=range States { clips[s]=Clip{4,100*time.Millisecond} }
 return Config{clips,10*time.Second,2}
}

func newTest(t *testing.T)*Engine {
 t.Helper();e,err:=New(testConfig());if err!=nil {t.Fatal(err)};return e
}

func TestDropEventLifecycleAndLateTicks(t *testing.T) {
 e:=newTest(t)
 e.Fire(0,FilesStored)
 // eating/recovery complete at 1.2 s; the countdown belongs to that event.
 e.Advance(11200*time.Millisecond)
 if s:=e.Snapshot();s.State!=ToBack||s.Frame!=0||s.DropVisible {t.Fatalf("turning start: %+v",s)}
 for _,at:=range []int{11300,11400,11500,11600,11700,11799} {
  e.Fire(time.Duration(at)*time.Millisecond,DoubleClick)
  if e.Snapshot().DropVisible {t.Fatalf("early object at %d",at)}
 }
 // Cross BeginDrop and several state boundaries in one timer update.
 e.Advance(12100*time.Millisecond)
 if s:=e.Snapshot();s.State!=FromBack||!s.DropVisible||s.DropAge!=300*time.Millisecond {t.Fatalf("late update: %+v",s)}
 begins:=0
 for _,ev:=range e.DrainEvents(){if ev.Kind=="BeginDrop"{begins++;if ev.At!=11800*time.Millisecond {t.Fatal(ev)}}}
 if begins!=1 {t.Fatalf("BeginDrop count %d",begins)}
 e.Advance(12800*time.Millisecond)
 if s:=e.Snapshot();s.State!=Idle||s.DropVisible {t.Fatalf("completed: %+v",s)}
 e.Advance(15000*time.Millisecond)
 if e.Snapshot().DropVisible {t.Fatal("old prop reappeared")}
}

func TestClicksHoverAndDigestPriority(t *testing.T) {
 e:=newTest(t)
 e.Fire(0,HoverOn);e.Advance(500*time.Millisecond)
 if e.Snapshot().State!=Fight {t.Fatal(e.Snapshot())}
 ok,_:=e.Fire(500*time.Millisecond,DoubleClick);if !ok {t.Fatal("fight must accept laughter")}
 if e.Snapshot().State!=FromFight {t.Fatal("fight must recover before laughter")}
 for at:=600;at<2100;at+=50 {
  ok,_:=e.Fire(time.Duration(at)*time.Millisecond,DoubleClick)
  if ok {t.Fatal("click restarted laughter")}
 }
 e.Fire(2100*time.Millisecond,HoverOff);e.Advance(2900*time.Millisecond)
 if e.Snapshot().State!=Idle {t.Fatal(e.Snapshot())}
 e.Fire(2900*time.Millisecond,FilesStored)
 e.Fire(3100*time.Millisecond,Contact)
 if e.Snapshot().State!=ToEat {t.Fatal("contact interrupted eating")}
 e.Advance(4100*time.Millisecond)
 e.Fire(4100*time.Millisecond,DoubleClick)
 e.Advance(14100*time.Millisecond)
 if s:=e.Snapshot();s.State!=ToBack||s.DropVisible {t.Fatal(s)}
}

func TestAllStateEntriesResetFrameAndClockRegression(t *testing.T) {
 e:=newTest(t);e.Fire(0,DoubleClick);e.Advance(800*time.Millisecond)
 if s:=e.Snapshot();s.State!=FromLaugh||s.Frame!=0 {t.Fatal(s)}
 before:=e.Snapshot()
 if e.Advance(700*time.Millisecond)==nil {t.Fatal("accepted clock regression")}
 if after:=e.Snapshot();after!=before {t.Fatal("regression changed state")}
}

func TestDropContinuousAndNoTunneling(t *testing.T) {
 c:=DropConfig{X:365,StartY:430,GroundY:525,InitialVelocity:0,Gravity:650,Restitution:.18,RevealTime:200*time.Millisecond,SquashTime:80*time.Millisecond,SettleSpeed:8}
 prev,_:=DropAt(c,0)
 for ms:=1;ms<3000;ms++ {
  p,err:=DropAt(c,time.Duration(ms)*time.Millisecond);if err!=nil {t.Fatal(err)}
  if p.X!=365||p.BottomY>525||math.Abs(p.BottomY-prev.BottomY)>1||math.Abs(p.ScaleY-prev.ScaleY)>.01 {t.Fatalf("trajectory discontinuity %+v -> %+v",prev,p)}
  prev=p
 }
 p,_:=DropAt(c,60*time.Second);if !p.Settled||p.BottomY!=525 {t.Fatal(p)}
}

func TestNoDropBeforeMarkerForRapidInteractions(t *testing.T) {
 e:=newTest(t);e.Fire(0,FilesStored)
 for ms:=0;ms<11800;ms+=7 {
  for _,input:=range []Input{HoverOn,DoubleClick,Contact,HoverOff} {e.Fire(time.Duration(ms)*time.Millisecond,input)}
  if e.Snapshot().DropVisible {t.Fatalf("early prop at %d",ms)}
 }
 e.Advance(20*time.Second)
 begins:=0
 for _,ev:=range e.DrainEvents() {
  if ev.Kind=="BeginDrop" {begins++;if ev.At<11800*time.Millisecond {t.Fatal("early marker",ev)}}
 }
 if begins!=1 {t.Fatalf("expected one eventual marker, got %d",begins)}
}
