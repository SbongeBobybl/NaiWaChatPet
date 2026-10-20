package emotion

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestSkillsStayOutOfTheEmotionSet(t *testing.T) {
	skills, err := Skills()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 3 {
		t.Fatalf("skills = %d, want 3", len(skills))
	}
	want := []string{"skill_fan_throw", "skill_roll", "skill_power_up"}
	choices, err := ForcedChoices()
	if err != nil {
		t.Fatal(err)
	}
	choiceSet := map[string]bool{}
	for _, id := range choices {
		choiceSet[id] = true
	}
	for i, id := range want {
		if skills[i].ID != id {
			t.Fatalf("skill[%d] = %s, want %s", i, skills[i].ID, id)
		}
		if _, ok := ByID(id); ok {
			t.Fatalf("%s is in the emotion catalog", id)
		}
		if choiceSet[id] {
			t.Fatalf("%s is in ForcedChoices", id)
		}
		if _, ok := SkillByID(id); !ok {
			t.Fatalf("SkillByID(%s) missing", id)
		}
		s := NewScheduler(DefaultDurations())
		loc := &fakeLoco{st: readyWalk()}
		if err := s.Start(loc); err != nil {
			t.Fatal(err)
		}
		d := s.RequestEmotion(id, PriorityNormal)
		if d.Reason != "unknown_emotion" || d.Accepted {
			t.Fatalf("RequestEmotion(%s) = %+v, want unknown_emotion", id, d)
		}
	}
	if _, ok := SkillByID("happy"); ok {
		t.Fatal("happy must not be a skill")
	}
}

func TestSkillClipsOpenOnReference(t *testing.T) {
	root := repoRoot(t)
	ref, err := os.ReadFile(filepath.Join(root, "source", "emotion", "neutral-reference.png"))
	if err != nil {
		t.Fatal(err)
	}
	refSum := sha256.Sum256(ref)
	wantHash := hex.EncodeToString(refSum[:])
	refImg := mustNRGBA(t, ref)
	skills, err := Skills()
	if err != nil {
		t.Fatal(err)
	}
	frames := map[string]int{
		"skill_fan_throw": 14,
		"skill_roll":      13,
		"skill_power_up":  13,
	}
	legacy := legacyFrameHashes(t, root)
	for _, sk := range skills {
		if sk.FPS < 1 || sk.FPS > 49 {
			t.Fatalf("%s fps %d", sk.ID, sk.FPS)
		}
		if frames[sk.ID] == 0 {
			t.Fatalf("unexpected skill %s", sk.ID)
		}
		if sk.FrameCount != frames[sk.ID] {
			t.Fatalf("%s frame_count %d, want %d", sk.ID, sk.FrameCount, frames[sk.ID])
		}
		dir := filepath.Join(root, filepath.FromSlash(sk.Clip))
		raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		var man struct {
			Name       string `json:"name"`
			Emotion    string `json:"emotion"`
			Skill      string `json:"skill"`
			Kind       string `json:"kind"`
			FPS        int    `json:"fps"`
			FrameCount int    `json:"frameCount"`
			Reviewed   bool   `json:"reviewed"`
			Frames     []struct {
				File   string `json:"file"`
				SHA256 string `json:"sha256"`
			} `json:"frames"`
		}
		if err := json.Unmarshal(raw, &man); err != nil {
			t.Fatal(err)
		}
		if man.Name != sk.ID || man.Skill != sk.ID || man.Kind != "skill" || man.Emotion != "" {
			t.Fatalf("%s manifest name=%s skill=%s kind=%s emotion=%q", sk.ID, man.Name, man.Skill, man.Kind, man.Emotion)
		}
		if !man.Reviewed || man.FPS != sk.FPS || man.FrameCount != sk.FrameCount || man.FrameCount != len(man.Frames) {
			t.Fatalf("%s manifest timing reviewed=%v fps=%d frames=%d", sk.ID, man.Reviewed, man.FPS, man.FrameCount)
		}
		pngs, err := filepath.Glob(filepath.Join(dir, "*.png"))
		if err != nil {
			t.Fatal(err)
		}
		if len(pngs) != sk.FrameCount {
			t.Fatalf("%s png count %d, want %d", sk.ID, len(pngs), sk.FrameCount)
		}
		for i, fr := range man.Frames {
			b, err := os.ReadFile(filepath.Join(dir, fr.File))
			if err != nil {
				t.Fatalf("%s %s: %v", sk.ID, fr.File, err)
			}
			sum := sha256.Sum256(b)
			got := hex.EncodeToString(sum[:])
			if got != fr.SHA256 {
				t.Fatalf("%s %s sha mismatch", sk.ID, fr.File)
			}
			if got != wantHash {
				if _, reused := legacy[got]; reused {
					t.Fatalf("%s %s reuses a legacy frame", sk.ID, fr.File)
				}
			}
			if i == 0 && got != wantHash {
				t.Fatalf("%s/0000.png is not the reference plate", sk.ID)
			}
			if sk.ID != "skill_power_up" && i == len(man.Frames)-1 && got != wantHash {
				t.Fatalf("%s closing frame is not the reference plate", sk.ID)
			}
			if sk.ID == "skill_power_up" {
				if i > 0 && got == wantHash {
					t.Fatalf("%s %s has no fire layer", sk.ID, fr.File)
				}
				assertOpaqueCharacterIntact(t, refImg, mustNRGBA(t, b), sk.ID+"/"+fr.File)
			}
		}
	}
}

func assertOpaqueCharacterIntact(t *testing.T, ref, frame *image.NRGBA, label string) {
	t.Helper()
	if ref.Bounds() != frame.Bounds() {
		t.Fatalf("%s bounds %v, reference %v", label, frame.Bounds(), ref.Bounds())
	}
	b := ref.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rc := ref.NRGBAAt(x, y)
			if rc.A != 255 {
				continue
			}
			if frame.NRGBAAt(x, y) != rc {
				t.Fatalf("%s repainted opaque character pixel at %d,%d", label, x, y)
			}
		}
	}
}

func mustNRGBA(t *testing.T, raw []byte) *image.NRGBA {
	t.Helper()
	im, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := im.(*image.NRGBA); ok {
		return n
	}
	out := image.NewNRGBA(im.Bounds())
	draw.Draw(out, out.Bounds(), im, im.Bounds().Min, draw.Src)
	return out
}

func legacyFrameHashes(t *testing.T, root string) map[string]struct{} {
	t.Helper()
	ems, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	dirs := map[string]bool{}
	for _, em := range ems {
		if !em.Legacy {
			continue
		}
		dirs[em.ID] = true
		dirs[em.TransitionIn] = true
		dirs[em.TransitionOut] = true
	}
	out := map[string]struct{}{}
	for dir := range dirs {
		frames, err := filepath.Glob(filepath.Join(root, "animations", dir, "*.png"))
		if err != nil {
			t.Fatal(err)
		}
		for _, frame := range frames {
			b, err := os.ReadFile(frame)
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(b)
			out[hex.EncodeToString(sum[:])] = struct{}{}
		}
	}
	return out
}
