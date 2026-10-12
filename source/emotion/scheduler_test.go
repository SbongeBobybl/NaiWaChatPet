package emotion

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestForcedSetCoversThirtyEmotions(t *testing.T) {
	choices, err := ForcedChoices()
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) < 31 { // neutral + at least 30 playable
		t.Fatalf("forced set has %d entries, want at least 31 including neutral", len(choices))
	}
	playable, err := PlayableIDs()
	if err != nil {
		t.Fatal(err)
	}
	if len(playable) < 30 {
		t.Fatalf("playable emotions = %d, want >= 30", len(playable))
	}
	need := []string{"angry", "happy", "confused", "surprised", "sad", "shy"}
	have := map[string]bool{}
	for _, id := range playable {
		have[id] = true
		if _, ok := ByID(id); !ok {
			t.Fatalf("playable id %s missing from catalog", id)
		}
	}
	for _, id := range need {
		if !have[id] {
			t.Fatalf("original emotion %s missing from forced set", id)
		}
	}
	if choices[0] != NeutralID {
		t.Fatalf("forced set must start with neutral, got %s", choices[0])
	}
}

func TestEachEmotionHasPlayableManifest(t *testing.T) {
	root := repoRoot(t)
	ems, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	filepath.Walk(filepath.Join(root, "animations"), func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && filepath.Ext(path) == ".png" {
			total += info.Size()
		}
		return nil
	})
	// Legacy loader rejects the library above 128MiB of frame bytes.
	if total > 125<<20 {
		t.Fatalf("animation png bytes %d exceed safe budget under 128MiB", total)
	}
	for _, em := range ems {
		manPath := filepath.Join(root, filepath.FromSlash(em.ManifestPath()))
		raw, err := os.ReadFile(manPath)
		if err != nil {
			t.Fatalf("%s: %v", em.ID, err)
		}
		var man struct {
			Name          string `json:"name"`
			Emotion       string `json:"emotion"`
			FPS           int    `json:"fps"`
			FrameCount    int    `json:"frameCount"`
			Reviewed      bool   `json:"reviewed"`
			TransitionIn  string `json:"transitionIn"`
			TransitionOut string `json:"transitionOut"`
			Frames        []struct {
				File   string `json:"file"`
				SHA256 string `json:"sha256"`
			} `json:"frames"`
		}
		if err := json.Unmarshal(raw, &man); err != nil {
			t.Fatalf("%s manifest: %v", em.ID, err)
		}
		if man.Name != em.ID || man.Emotion != em.ID {
			t.Fatalf("%s manifest name/emotion = %s/%s", em.ID, man.Name, man.Emotion)
		}
		if !man.Reviewed || man.FPS < 1 || man.FPS > 49 || man.FrameCount != len(man.Frames) || man.FrameCount < 1 {
			t.Fatalf("%s invalid timing reviewed=%v fps=%d frames=%d", em.ID, man.Reviewed, man.FPS, man.FrameCount)
		}
		if man.TransitionIn != em.TransitionIn || man.TransitionOut != em.TransitionOut {
			t.Fatalf("%s transitions manifest=%s/%s catalog=%s/%s", em.ID, man.TransitionIn, man.TransitionOut, em.TransitionIn, em.TransitionOut)
		}
		for _, tr := range []string{man.TransitionIn, man.TransitionOut} {
			if tr == "" {
				t.Fatalf("%s missing transition", em.ID)
			}
			if _, err := os.Stat(filepath.Join(root, "animations", tr, "manifest.json")); err != nil {
				t.Fatalf("%s transition %s: %v", em.ID, tr, err)
			}
		}
		dir := filepath.Join(root, filepath.FromSlash(em.Clip))
		for _, fr := range man.Frames {
			p := filepath.Join(dir, fr.File)
			b, err := os.ReadFile(p)
			if err != nil {
				t.Fatalf("%s frame %s: %v", em.ID, fr.File, err)
			}
			sum := sha256.Sum256(b)
			if hex.EncodeToString(sum[:]) != fr.SHA256 {
				t.Fatalf("%s frame %s sha mismatch", em.ID, fr.File)
			}
		}
	}
}

func TestLegacyClipsUntouched(t *testing.T) {
	root := repoRoot(t)
	want := map[string]int{"angry": 42, "confused": 95, "sad": 120, "shy": 95, "surprised": 95}
	for id, n := range want {
		var man struct {
			FrameCount int `json:"frameCount"`
		}
		b, err := os.ReadFile(filepath.Join(root, "animations", id, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, &man); err != nil {
			t.Fatal(err)
		}
		if man.FrameCount != n {
			t.Fatalf("%s frameCount=%d want %d", id, man.FrameCount, n)
		}
	}
}

type fakeLoco struct {
	st   LocomotionState
	subs []func(LocomotionState)
}

func (f *fakeLoco) GetLocomotionState() LocomotionState { return f.st }
func (f *fakeLoco) OnLocomotionChange(fn func(LocomotionState)) func() {
	f.subs = append(f.subs, fn)
	return func() {}
}
func (f *fakeLoco) set(st LocomotionState) {
	f.st = st
	for _, fn := range f.subs {
		fn(st)
	}
}

func readyWalk() LocomotionState {
	return LocomotionState{ActionName: "walk", Grounded: true, Facing: 1, CanPlayEmotion: true}
}

func TestStartupOrderAndBusyQueue(t *testing.T) {
	s := NewScheduler(Durations{InMS: 100, LoopMS: 200, OutMS: 100})
	loc := &fakeLoco{st: LocomotionState{ActionName: "jump", Grounded: false, BusyLocomotion: true}}
	if err := s.Start(loc); err != nil {
		t.Fatal(err)
	}
	d := s.RequestEmotion("happy", PriorityNormal)
	if d.Reason != "not_ready" || d.Accepted || d.Queued {
		t.Fatalf("before grounded ready: %+v", d)
	}
	loc.set(readyWalk())
	if !s.ready {
		t.Fatal("scheduler did not become ready after grounded locomotion")
	}
	d = s.RequestEmotion("love", PriorityNormal)
	if !d.Accepted || s.GetEmotionState().EmotionPhase != PhaseIn {
		t.Fatalf("walk overlay: %+v state=%+v", d, s.GetEmotionState())
	}
	if !s.GetEmotionState().EmotionBlocksHoverPunch || !s.GetEmotionState().EmotionPlaying {
		t.Fatalf("hover punch should be blocked while playing: %+v", s.GetEmotionState())
	}
	loc.set(LocomotionState{ActionName: "attack", Grounded: false, BusyLocomotion: true, CanPlayEmotion: true})
	if s.GetEmotionState().EmotionPlaying {
		t.Fatalf("attack should clear overlay, state=%+v", s.GetEmotionState())
	}
	loc.set(readyWalk())
	st := s.GetEmotionState()
	if st.EmotionPhase != PhaseIn || st.EmotionID != "love" {
		t.Fatalf("emotion_in should resume after attack, state=%+v", st)
	}
}

func TestHighPriorityNeverInterruptsJump(t *testing.T) {
	s := NewScheduler(DefaultDurations())
	loc := &fakeLoco{st: LocomotionState{ActionName: "jump", Grounded: false, BusyLocomotion: true, CanPlayEmotion: true}}
	// Become ready first, then enter jump.
	loc.st = readyWalk()
	if err := s.Start(loc); err != nil {
		t.Fatal(err)
	}
	loc.set(LocomotionState{ActionName: "jump", Grounded: false, BusyLocomotion: true, CanPlayEmotion: true})
	d := s.RequestEmotion("shocked", PriorityHigh)
	if !d.Queued || d.Accepted {
		t.Fatalf("high emotion during jump should queue, got %+v", d)
	}
	if s.InterruptsLocomotionIdle() {
		t.Fatal("high emotion must not interrupt jump")
	}
	loc.set(LocomotionState{ActionName: "idle", Grounded: true, CanPlayEmotion: true})
	st := s.GetEmotionState()
	if st.EmotionID != "shocked" || st.EmotionPhase != PhaseIn || !s.InterruptsLocomotionIdle() {
		t.Fatalf("queued high emotion should enter in on idle, state=%+v interrupt=%v", st, s.InterruptsLocomotionIdle())
	}
}

func TestManualBeatsAutoAndPhasesAdvance(t *testing.T) {
	s := NewScheduler(Durations{InMS: 50, LoopMS: 50, OutMS: 50})
	loc := &fakeLoco{st: readyWalk()}
	if err := s.Start(loc); err != nil {
		t.Fatal(err)
	}
	var changes int
	s.OnEmotionChange(func(EmotionState) { changes++ })
	if d := s.RequestEmotion("bored", PriorityLow); !d.Accepted {
		t.Fatal(d)
	}
	if d := s.RequestEmotion("sleepy", PriorityLow); d.Reason != "lower_priority" {
		t.Fatalf("same rank auto should not replace, got %+v", d)
	}
	if s.GetEmotionState().EmotionID != "bored" {
		t.Fatalf("same rank interrupted, state=%+v", s.GetEmotionState())
	}
	d := s.RequestEmotion("crying", PriorityHigh)
	if !d.Accepted || s.GetEmotionState().EmotionID != "crying" {
		t.Fatalf("high should interrupt low auto, %+v state=%+v", d, s.GetEmotionState())
	}
	d = s.RequestEmotionFrom("hug", PriorityLow, CauseManual)
	if !d.Accepted || s.GetEmotionState().EmotionID != "hug" {
		t.Fatalf("manual should beat auto, %+v state=%+v", d, s.GetEmotionState())
	}
	d = s.RequestEmotion("celebrate", PriorityHigh)
	if d.Accepted || d.Reason != "lower_priority" {
		t.Fatalf("auto should not beat manual, %+v", d)
	}
	s.Tick(50)
	if s.GetEmotionState().EmotionPhase != PhaseLoop {
		t.Fatalf("phase after in: %+v", s.GetEmotionState())
	}
	s.Tick(50)
	if s.GetEmotionState().EmotionPhase != PhaseOut {
		t.Fatalf("phase after loop: %+v", s.GetEmotionState())
	}
	s.Tick(50)
	st := s.GetEmotionState()
	if st.EmotionPlaying || st.EmotionPhase != PhaseIdle || st.EmotionBlocksHoverPunch {
		t.Fatalf("should return idle: %+v", st)
	}
	if changes < 4 {
		t.Fatalf("onEmotionChange fired %d times", changes)
	}
}

func TestRunAllowsOverlay(t *testing.T) {
	s := NewScheduler(DefaultDurations())
	loc := &fakeLoco{st: LocomotionState{ActionName: "run", Grounded: true, Facing: -1, CanPlayEmotion: true}}
	if err := s.Start(loc); err != nil {
		t.Fatal(err)
	}
	d := s.RequestEmotion("greet", PriorityNormal)
	if !d.Accepted || !s.GetEmotionState().EmotionPlaying {
		t.Fatalf("run should allow overlay: %+v %+v", d, s.GetEmotionState())
	}
	if s.InterruptsLocomotionIdle() {
		t.Fatal("normal priority must not claim locomotion idle interrupt")
	}
}

func TestUnknownEmotionRejected(t *testing.T) {
	s := NewScheduler(DefaultDurations())
	loc := &fakeLoco{st: readyWalk()}
	if err := s.Start(loc); err != nil {
		t.Fatal(err)
	}
	d := s.RequestEmotion("not-a-real-emotion", PriorityNormal)
	if d.Reason != "unknown_emotion" {
		t.Fatalf("got %+v", d)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
