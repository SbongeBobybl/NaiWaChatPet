package companion

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChangingProviderClearsCredentialsAndHistory(t *testing.T) {
	s := serverWithMock(t, &Client{})
	s.history = []Message{{Role: "user", Content: "private"}, {Role: "assistant", Content: "private"}}
	r := httptest.NewRequest("POST", "/api/settings", bytes.NewBufferString(`{"modelChoice":"custom","modelId":"another-model","baseUrl":"https://another.example/v1"}`))
	w := httptest.NewRecorder()
	s.saveSettings(w, r)
	if w.Code != 200 || s.key != "" || len(s.history) != 0 || s.settings.ScreenEnabled {
		t.Fatalf("provider change retained secret or history: status=%d", w.Code)
	}
}

func TestOfficialModelsAndCustomValidation(t *testing.T) {
	p, e := ModelFromSettings(Settings{})
	if e != nil || p.ID != "deepseek-flash" || !p.Vision || p.Endpoint != deepSeekEndpoint {
		t.Fatalf("%+v %v", p, e)
	}
	p, e = ModelFromSettings(Settings{ModelChoice: "pro"})
	if e != nil || p.ID != "deepseek-v4-pro" || p.Vision {
		t.Fatalf("%+v %v", p, e)
	}
	for _, s := range []Settings{{BaseURL: "https://user:secret@example.com"}, {BaseURL: "http://remote.example"}, {BaseURL: "https://api.deepseek.com?key=secret"}, {ModelChoice: "custom", ModelID: "not valid"}, {ModelChoice: "custom", ModelID: "compatible", Thinking: true}} {
		if _, e := ModelFromSettings(s); e == nil {
			t.Fatal("unsafe/unconfirmed config accepted", s.ModelChoice)
		}
	}
	p, e = ModelFromSettings(Settings{ModelChoice: "custom", ModelID: "vendor-model", BaseURL: "http://127.0.0.1:8000/v1", CustomVision: true})
	if e != nil || p.ThinkingExtension || !p.Vision {
		t.Fatalf("%+v %v", p, e)
	}
}
func TestNonStreamingAndThinkingConfiguration(t *testing.T) {
	c, api := mockClient(func(w http.ResponseWriter, r *http.Request) {
		var p chatRequest
		_ = json.NewDecoder(r.Body).Decode(&p)
		if p.Stream || p.Thinking["type"] != "enabled" {
			t.Error("configured mode not respected", p.Stream, p.Thinking)
		}
		w.Write([]byte(normalReply))
	}, 1)
	defer api.Close()
	_, e := c.Complete(context.Background(), "test-credential-config", RequestOptions{Model: "deepseek-flash", Thinking: true, MaxTokens: 8192}, []Message{{Role: "user", Content: "test"}}, nil)
	if e != nil {
		t.Fatal(e)
	}
}
func TestPNGImageContentAndUnsupportedVision(t *testing.T) {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewNRGBA(image.Rect(0, 0, 2, 2)))
	m, e := makeImageContent("this image", b.Bytes())
	if e != nil || m[1]["type"] != "image_url" {
		t.Fatal(e)
	}
	c, api := mockClient(func(w http.ResponseWriter, r *http.Request) { t.Error("unsupported model requested") }, 1)
	defer api.Close()
	s := serverWithMock(t, c)
	s.mu.Lock()
	s.settings.ModelChoice = "pro"
	s.model = "deepseek-v4-pro"
	s.mu.Unlock()
	if _, e := s.begin(context.Background(), chatBody{Text: "image", WithScreen: true, ConfirmScreen: true}); e == nil {
		t.Fatal("vision mismatch not rejected")
	}
}
