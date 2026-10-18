package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"codexpet/reconstruction/animation"
)

func testConfig() runtimeJSON {
	c := runtimeJSON{Version: 1, Canvas: canvasJSON{4, 4}, Clips: map[string]clipJSON{}, DigestDelayNS: int64(100 * time.Millisecond), BeginDropFrame: 2, BodyDir: "body", HoverMask: "mask.png", Prop: "prop.png", Scale: scaleJSON{Default: .5, Min: .26, Max: .65, Step: .035}, Drop: dropJSON{X: 2, StartY: 1, GroundY: 3, InitialVelocity: 0, Gravity: 10, Restitution: .2, SettleSpeed: 1, RevealNS: int64(10 * time.Millisecond), SquashNS: int64(20 * time.Millisecond)}, Gate: gateJSON{Passed: true}, Hashes: map[string]string{}}
	for _, s := range animation.States {
		c.Clips[string(s)] = clipJSON{Frames: 3, DurationNS: int64(10 * time.Millisecond)}
	}
	c.Clips[string(animation.Poop)] = clipJSON{Frames: 12, DurationNS: int64(10 * time.Millisecond)}
	return c
}
func testEngine(t *testing.T) *animation.Engine {
	t.Helper()
	cfg, err := testConfig().validate()
	if err != nil {
		t.Fatal(err)
	}
	e, err := animation.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestOpaqueLegacyMaskUsesLuminance(t *testing.T) {
	m := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	m.SetNRGBA(0, 0, color.NRGBA{A: 255})
	m.SetNRGBA(1, 0, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	n := normalizedMask(m, "auto")
	if maskHit(n, 0, 0, 2, 1) {
		t.Fatal("opaque black background must not become a rectangle hit")
	}
	if !maskHit(n, 1, 0, 2, 1) {
		t.Fatal("white silhouette must hit")
	}
	if m.NRGBAAt(0, 0).A != 255 {
		t.Fatal("conversion must not mutate source")
	}
}
func TestHoverHoldAndThreshold(t *testing.T) {
	var h hoverHold
	if !h.update(0, true) || !h.update(250*time.Millisecond, false) || h.update(251*time.Millisecond, false) {
		t.Fatal("250 ms leave hold violated")
	}
	m := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	m.SetNRGBA(0, 0, color.NRGBA{A: 40})
	m.SetNRGBA(1, 0, color.NRGBA{A: 41})
	if maskHit(m, 0, 0, 2, 1) || !maskHit(m, 1, 0, 2, 1) || maskHit(m, -1, 0, 2, 1) {
		t.Fatal("strict alpha > 40 or bounds violated")
	}
}
func TestStoredActionsRetainedDuringAtomicAndDigestion(t *testing.T) {
	e := testEngine(t)
	if _, err := e.Fire(0, animation.FilesStored); err != nil {
		t.Fatal(err)
	}
	q := actionQueue{}
	q.stored()
	q.stored()
	if ok, err := q.service(e, 0); err != nil || ok || q.storedBatches != 2 {
		t.Fatalf("pending storage lost while atomic: %#v %v", q, err)
	}
	if err := e.Advance(100 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if ok, err := q.service(e, 100*time.Millisecond); err != nil || ok || q.storedBatches != 2 {
		t.Fatal("pending storage lost while digestion scheduled")
	}
	for step := 1; step <= 2; step++ {
		now := time.Duration(step) * time.Second
		if err := e.Advance(now); err != nil {
			t.Fatal(err)
		}
		ok, err := q.service(e, now)
		if err != nil || !ok || q.storedBatches != 2-step {
			t.Fatalf("stored notification must be removed exactly on acceptance: %#v %v", q, err)
		}
	}
}
func TestFightExitsThroughBridgeBeforeStoredAction(t *testing.T) {
	e := testEngine(t)
	e.Fire(0, animation.HoverOn)
	e.Advance(50 * time.Millisecond)
	if e.Snapshot().State != animation.Fight {
		t.Fatal("setup")
	}
	q := actionQueue{}
	q.stored()
	q.service(e, 50*time.Millisecond)
	if q.storedBatches != 1 || e.Snapshot().State != animation.FromFight {
		t.Fatal("bridge or retained notification missing")
	}
	e.Advance(80 * time.Millisecond)
	accepted, err := q.service(e, 80*time.Millisecond)
	if err != nil || !accepted || q.storedBatches != 0 || e.Snapshot().State != animation.ToEat {
		t.Fatal("file action was lost after bridge")
	}
}
func TestContactLatchRequiresSeparation(t *testing.T) {
	a := rectJSON{0, 0, 100, 100}
	p := []rectJSON{{20, 20, 100, 100}}
	var latch contactLatch
	if !latch.update(a, p) || latch.update(a, p) || latch.update(a, nil) || !latch.update(a, p) {
		t.Fatal("contact must trigger only on rising overlap")
	}
	if overlaps(a, rectJSON{99, 0, 100, 100}) {
		t.Fatal("one-pixel edge must not trigger contact")
	}
}
func TestResizePreservesActualGroundAnchor(t *testing.T) {
	w, h, err := scaledCanvas(image.Rect(0, 0, 420, 560), .5*144/96)
	if err != nil || w != 315 || h != 420 {
		t.Fatalf("common DPI transform: %d %d %v", w, h, err)
	}
	oldFactor, newFactor, sourceGround := .365*96/96, .5*144/96, 540.0
	y := scaleAnchor(100, sourceGround, oldFactor, newFactor)
	oldGround := 100 + int(math.Round(sourceGround*oldFactor))
	newGround := y + int(math.Round(sourceGround*newFactor))
	if oldGround != newGround {
		t.Fatalf("actual ground anchor moved: %d -> %d", oldGround, newGround)
	}
	if y == scaleBottom(100, 204, h) {
		t.Fatal("test must distinguish ground anchor from transparent window bottom")
	}
}
func TestPeerProtocolStalenessAndFutureRejected(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1000, 0)
	owners := []string{"123-aaaaaaaaaaaaaaaaaaaaaaaa", "124-bbbbbbbbbbbbbbbbbbbbbbbb", "125-cccccccccccccccccccccccc"}
	for i, owner := range owners {
		stamp := now
		if i == 1 {
			stamp = now.Add(-6 * time.Second)
		}
		if i == 2 {
			stamp = now.Add(time.Second)
		}
		p := peerRecord{peerProtocol, owner, 123, stamp.UnixNano(), rectJSON{0, 0, 100, 100}}
		b, _ := json.Marshal(p)
		if err := os.WriteFile(filepath.Join(dir, owner+".json"), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	peers, err := readPeers(dir, "self", now)
	if err != nil || len(peers) != 1 {
		t.Fatalf("invalid/stale peers accepted: %v %v", peers, err)
	}
}
func TestUniqueDestinationDoesNotReuseNames(t *testing.T) {
	dir := t.TempDir()
	seen := map[string]bool{}
	at := time.Unix(1000, 0)
	for i := 0; i < 30; i++ {
		p, err := uniqueTarget(dir, "same.txt", at)
		if err != nil || seen[p] {
			t.Fatalf("target not unique: %s %v", p, err)
		}
		seen[p] = true
		if filepath.Dir(p) != dir {
			t.Fatal("target escaped storage")
		}
	}
}
func TestTransferProblemMessageExplainsCopyStateInChinese(t *testing.T) {
	text := transferProblemsText([]transferProblem{{Source: `C:\drop\a.txt`, Target: `C:\Received\a.txt`, TargetVerified: true}})
	if !strings.Contains(text, "\u76ee\u6807\u526f\u672c\u5df2\u901a\u8fc7\u6821\u9a8c") || !strings.Contains(text, "\u4e24\u4efd\u6587\u4ef6\u90fd\u5728") || !strings.Contains(text, `C:\drop\a.txt`) || !strings.Contains(text, `C:\Received\a.txt`) {
		t.Fatalf("user message must state both paths and the verified-copy result: %s", text)
	}
}
func imagePNG(t *testing.T, im image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func testBundle(t *testing.T) (fstest.MapFS, runtimeJSON) {
	t.Helper()
	c := testConfig()
	files := fstest.MapFS{}
	body := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	body.SetNRGBA(1, 1, color.NRGBA{R: 20, G: 30, B: 40, A: 255})
	data := imagePNG(t, body)
	put := func(name string, b []byte) {
		sum := sha256.Sum256(b)
		c.Hashes[name] = hex.EncodeToString(sum[:])
		files[name] = &fstest.MapFile{Data: b}
	}
	for _, s := range animation.States {
		for i := 0; i < c.Clips[string(s)].Frames; i++ {
			name := filepath.ToSlash(filepath.Join(c.BodyDir, fmtFrame(string(s), i)))
			put(name, data)
		}
	}
	mask := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	for i := 3; i < len(mask.Pix); i += 4 {
		mask.Pix[i] = 255
	}
	mask.SetNRGBA(1, 1, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	put(c.HoverMask, imagePNG(t, mask))
	put(c.Prop, data)
	config, _ := json.Marshal(c)
	files["runtime.json"] = &fstest.MapFile{Data: config}
	return files, c
}
func fmtFrame(s string, i int) string {
	return s + "_" + string(rune('0'+i/100)) + string(rune('0'+i/10%10)) + string(rune('0'+i%10)) + ".png"
}
func TestReleaseGateAndExactHashes(t *testing.T) {
	files, c := testBundle(t)
	loaded, err := loadAssets(files, "runtime.json")
	if err != nil || loaded == nil {
		t.Fatal(err)
	}
	if maskHit(loaded.Mask, 0, 0, 4, 4) {
		t.Fatal("legacy rectangle hover bug")
	}
	c.Gate.Passed = false
	c.Gate.Reason = "not reviewed"
	b, _ := json.Marshal(c)
	files["runtime.json"].Data = b
	if _, err = loadAssets(files, "runtime.json"); err == nil {
		t.Fatal("unreleased assets must be rejected")
	}
	c.Gate.Passed = true
	b, _ = json.Marshal(c)
	files["runtime.json"].Data = b
	files["body/idle_000.png"].Data = append([]byte{}, files["body/idle_000.png"].Data...)
	files["body/idle_000.png"].Data[20] ^= 1
	if _, err = loadAssets(files, "runtime.json"); err == nil {
		t.Fatal("changed body pixels must fail the release hash before GUI")
	}
}
func TestMissingFrameAndPathEscapeFailBeforeHost(t *testing.T) {
	files, _ := testBundle(t)
	delete(files, "body/toBack_001.png")
	if _, err := loadAssets(files, "runtime.json"); err == nil {
		t.Fatal("incomplete resource clip accepted")
	}
	if _, err := loadAssets(files, "../runtime.json"); err == nil {
		t.Fatal("asset traversal accepted")
	}
}
