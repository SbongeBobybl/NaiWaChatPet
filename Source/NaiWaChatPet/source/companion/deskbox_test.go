package companion

import (
	ai "codexpet/reconstruction/aiorganization"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDeskBoxToleratesExtraModelFields(t *testing.T) {
	content := "```json\n{\"schemaVersion\":1,\"requestId\":\"request-2\",\"summary\":\"ok\",\"items\":[{\"itemId\":\"f1\",\"categoryId\":\"study\",\"confidence\":0.8,\"reason\":\"学习/资料：课程\",\"note\":\"extra\"}],\"intents\":[],\"categories\":[]}\n```"
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}, "finish_reason": "stop"}}})
	}))
	defer vendor.Close()
	s := &Server{key: "fake-key", client: &Client{HTTP: vendor.Client()}, settings: Settings{ModelChoice: "custom", ModelID: "test", BaseURL: vendor.URL}}
	a := DeskBoxAgent{Server: s}
	r := ai.Request{SchemaVersion: 1, RequestID: "request-2", Operation: "classify", Consent: true,
		Items:      []ai.Item{{ItemID: "f1", DisplayName: "课程.pdf", Extension: ".pdf", Kind: "file"}},
		Categories: []ai.Category{{ID: "study", DisplayName: "学习"}}}
	result, err := a.Classify(context.Background(), r)
	if err != nil || len(result.Items) != 1 || result.Items[0].Reason != "学习/资料：课程" {
		t.Fatalf("tolerant model parsing failed: %v %+v", err, result)
	}
}

func TestDeskBoxConsentPrivacyAndSchema(t *testing.T) {
	var calls atomic.Int32
	vendor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		for _, secret := range []string{"private history", "fake-key", "LOCAL-IPC-SECRET", "LOCAL-SESSION", "C:\\Users"} {
			if strings.Contains(string(body), secret) {
				t.Errorf("leaked %s", secret)
			}
		}
		result := `{"schemaVersion":1,"requestId":"request-1","items":[{"itemId":"f1","categoryId":"study","confidence":0.8,"reason":"学习资料"}],"intents":[]}`
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": result}, "finish_reason": "stop"}}})
	}))
	defer vendor.Close()
	s := &Server{key: "fake-key", client: &Client{HTTP: vendor.Client()}, settings: Settings{ModelChoice: "custom", ModelID: "test", BaseURL: vendor.URL}, history: []Message{{Role: "user", Content: "private history"}}}
	a := DeskBoxAgent{Server: s}
	r := ai.Request{SchemaVersion: 1, RequestID: "request-1", Session: "LOCAL-SESSION", Token: "LOCAL-IPC-SECRET", Operation: "classify", Items: []ai.Item{{ItemID: "f1", DisplayName: "课程.pdf", Extension: ".pdf", Kind: "file"}}, Categories: []ai.Category{{ID: "study", DisplayName: "学习"}}}
	if _, err := a.Classify(context.Background(), r); err == nil || calls.Load() != 0 {
		t.Fatal("consent bypass")
	}
	r.Consent = true
	if result, err := a.Classify(context.Background(), r); err != nil || len(result.Items) != 1 || calls.Load() != 1 {
		t.Fatal(result, err)
	}
	if len(s.history) != 1 || s.key != "fake-key" {
		t.Fatal("chat state changed")
	}
	r.Items[0].IsProtected = true
	if _, err := a.Classify(context.Background(), r); err == nil || calls.Load() != 1 {
		t.Fatal("protection bypass")
	}
	r.Items[0].IsProtected = false
	r.Items[0].DisplayName = "password.txt"
	if _, err := a.Classify(context.Background(), r); err == nil || calls.Load() != 1 {
		t.Fatal("sensitive name sent")
	}
}
