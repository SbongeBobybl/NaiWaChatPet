package emotion

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image/png"
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

func TestLegacyPlatesUnchanged(t *testing.T) {
	root := repoRoot(t)
	want := map[string]string{
		"angry":     "1d0b804dc9f5f58fdec1b9e1da5dae3e3c20472f0320b5fd6339f024c5e2a909",
		"confused":  "af6c414e62f18641b5da34079355306b4b3a65bb69dcfc6162316f5f0eabce8a",
		"sad":       "51debe4d6dd13a65945a7397b2b4977873c5879fe4e8634c52bc79ec1c0427ff",
		"shy":       "97fc3fc1599e0a35671677a7e0d730f09ac7e11d808220ffd88b6e56ec80ced3",
		"surprised": "8a1032f8582d099bad7218a8012d1c38fe0685dbcaabe8257b4d951a99124c2b",
	}
	for id, sum := range want {
		b, err := os.ReadFile(filepath.Join(root, "animations", id, "0000.png"))
		if err != nil {
			t.Fatal(err)
		}
		got := sha256.Sum256(b)
		if hex.EncodeToString(got[:]) != sum {
			t.Fatalf("%s/0000.png changed", id)
		}
	}
}

func TestNewPerformancesAreNotLegacyCrops(t *testing.T) {
	root := repoRoot(t)
	ems, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	legacyDir := map[string]bool{}
	for _, em := range ems {
		if em.Legacy {
			legacyDir[em.ID] = true
			legacyDir[em.TransitionIn] = true
			legacyDir[em.TransitionOut] = true
		}
	}
	legacySHA := map[string]struct{}{}
	anim := filepath.Join(root, "animations")
	entries, err := os.ReadDir(anim)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !legacyDir[entry.Name()] {
			continue
		}
		frames, err := filepath.Glob(filepath.Join(anim, entry.Name(), "*.png"))
		if err != nil {
			t.Fatal(err)
		}
		for _, frame := range frames {
			b, err := os.ReadFile(frame)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(b)
			legacySHA[hex.EncodeToString(sum[:])] = struct{}{}
		}
	}
	sigs := map[string][]float64{}
	for _, em := range ems {
		if em.Legacy {
			if em.Source != "" || em.Count != 0 {
				t.Fatalf("legacy %s still has crop fields", em.ID)
			}
			continue
		}
		if em.Source != "" || em.Count != 0 || em.Step != 0 {
			t.Fatalf("%s still describes a legacy crop", em.ID)
		}
		if em.Production == "" {
			t.Fatalf("%s missing production method", em.ID)
		}
		if em.TransitionIn != em.ID+"_in" || em.TransitionOut != em.ID+"_out" {
			t.Fatalf("%s should own its transitions, got %s %s", em.ID, em.TransitionIn, em.TransitionOut)
		}
		for _, dir := range []string{em.ID, em.ID + "_in", em.ID + "_out"} {
			frames, err := filepath.Glob(filepath.Join(anim, dir, "*.png"))
			if err != nil || len(frames) == 0 {
				t.Fatalf("%s: %v frames=%d", dir, err, len(frames))
			}
			for _, frame := range frames {
				b, err := os.ReadFile(frame)
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(b)
				if _, ok := legacySHA[hex.EncodeToString(sum[:])]; ok {
					t.Fatalf("%s reuses a legacy frame", frame)
				}
			}
		}
		sigs[em.ID] = colorSig(t, filepath.Join(anim, em.ID, "0004.png"))
	}
	var closestA, closestB string
	closest := 9.0
	ids := make([]string, 0, len(sigs))
	for id := range sigs {
		ids = append(ids, id)
	}
	for i, a := range ids {
		for _, b := range ids[i+1:] {
			d := meanAbs(sigs[a], sigs[b])
			if d < closest {
				closest = d
				closestA, closestB = a, b
			}
		}
	}
	// Coarse 18x24 samples. Identical plates score 0. Distinct performances
	// on this rig stay above ~0.01; see contact-sheet.png for the glance test.
	if closest < 0.008 {
		t.Fatalf("mid-frames too alike: %s vs %s distance %.4f", closestA, closestB, closest)
	}
	t.Logf("closest new mid-frames %s vs %s distance %.4f", closestA, closestB, closest)
}

func colorSig(t *testing.T, path string) []float64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	im, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := im.Bounds()
	const gw, gh = 18, 24
	sig := make([]float64, 0, gw*gh*4)
	for y := 0; y < gh; y++ {
		for x := 0; x < gw; x++ {
			px := b.Min.X + (x*b.Dx())/gw + b.Dx()/(gw*2)
			py := b.Min.Y + (y*b.Dy())/gh + b.Dy()/(gh*2)
			r, g, bl, a := im.At(px, py).RGBA()
			sig = append(sig, float64(r)/65535, float64(g)/65535, float64(bl)/65535, float64(a)/65535)
		}
	}
	return sig
}

func meanAbs(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var sum float64
	for i := range a {
		d := a[i] - b[i]
		if d < 0 {
			d = -d
		}
		sum += d
	}
	return sum / float64(len(a))
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
