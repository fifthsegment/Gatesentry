package gatesentry2filters

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClassifyImageLocalHTTPIntegration(t *testing.T) {
	image := []byte("integration-image")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/chat" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing authorization")
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["model"] != "vision-test" || body["format"] != "json" || body["stream"] != false {
			t.Fatalf("request metadata = %#v", body)
		}
		messages := body["messages"].([]interface{})
		message := messages[0].(map[string]interface{})
		if message["images"].([]interface{})[0] != base64.StdEncoding.EncodeToString(image) {
			t.Error("image bytes changed in transit")
		}
		if !strings.Contains(message["content"].(string), "household content filter") {
			t.Error("classification prompt missing")
		}
		_, _ = io.WriteString(w, `{"message":{"content":"{\"nsfw\":true,\"confidence\":91,\"reason\":\"test\"}"}}`)
	}))
	defer server.Close()
	verdict, err := ClassifyImage(context.Background(), VisionRequest{Provider: AIModeLocal, APIKey: "test-key", BaseURL: server.URL, Model: "vision-test", ContentType: "image/png", Image: image, Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	if !verdict.ShouldBlock(90) || verdict.ShouldBlock(95) || verdict.Reason != "test" {
		t.Fatalf("verdict = %+v", verdict)
	}
}
