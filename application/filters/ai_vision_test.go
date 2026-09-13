package gatesentry2filters

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseImageVerdictJSON(t *testing.T) {
	v, err := ParseImageVerdictJSON("```json\n{\"nsfw\": true, \"confidence\": 91, \"reason\": \"nudity\"}\n```")
	if err != nil {
		t.Fatal(err)
	}
	if !v.NSFW || v.Confidence != 91 {
		t.Fatalf("%+v", v)
	}
	if !v.ShouldBlock(80) {
		t.Fatal("should block")
	}
	if v.ShouldBlock(95) {
		t.Fatal("below threshold")
	}
	wrapped, err := ParseImageVerdictJSON("thinking...\n{\"nsfw\": false, \"confidence\": 10, \"reason\": \"ok\"}")
	if err != nil || wrapped.NSFW || wrapped.Confidence != 10 {
		t.Fatalf("wrapped json: %+v err=%v", wrapped, err)
	}
}

func TestClassifyImageMockFlagsNSFW(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"message": map[string]string{
				"content": `{"nsfw": true, "confidence": 97, "reason": "explicit"}`,
			},
		})
	}))
	defer srv.Close()

	v, err := ClassifyImage(context.Background(), VisionRequest{
		Provider:    AIModeLocal,
		BaseURL:     srv.URL,
		Model:       "llava",
		ContentType: "image/jpeg",
		Image:       []byte("fake-jpeg-bytes-not-empty"),
		Client:      srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !v.NSFW || v.Confidence != 97 {
		t.Fatalf("%+v", v)
	}
}

func TestLiveProvidersFlagPornographicImage(t *testing.T) {
	loadAIDotEnv(t)
	imgPath := strings.TrimSpace(os.Getenv("GS_AI_TEST_IMAGE"))
	if imgPath == "" {
		imgPath = "/home/jbarwick/Development/33091992_018_4cb3.jpg"
	}
	img, err := os.ReadFile(imgPath)
	if err != nil {
		t.Skipf("test image not available: %v", err)
	}

	cases := []struct {
		name     string
		provider string
		keyEnv   string
		urlEnv   string
		modelEnv string
	}{
		{name: "grok", provider: AIModeGrok, keyEnv: "GS_AI_GROK_API_KEY", modelEnv: "GS_AI_GROK_MODEL"},
		{name: "chatgpt", provider: AIModeChatGPT, keyEnv: "GS_AI_OPENAI_API_KEY", modelEnv: "GS_AI_OPENAI_MODEL"},
		{name: "ollama", provider: AIModeLocal, urlEnv: "GS_AI_OLLAMA_URL", modelEnv: "GS_AI_OLLAMA_MODEL"},
	}

	ran := 0
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := VisionRequest{
				Provider:    tc.provider,
				APIKey:      os.Getenv(tc.keyEnv),
				BaseURL:     os.Getenv(tc.urlEnv),
				Model:       os.Getenv(tc.modelEnv),
				ContentType: "image/jpeg",
				Image:       img,
			}
			if tc.provider == AIModeLocal {
				if EnvLooksUnset(req.BaseURL) {
					t.Skip("GS_AI_OLLAMA_URL not set")
				}
			} else if EnvLooksUnset(req.APIKey) {
				t.Skipf("%s not set", tc.keyEnv)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			v, err := ClassifyImage(ctx, req)
			if err != nil {
				msg := err.Error()
				if strings.Contains(msg, "does not support image input") || strings.Contains(msg, "Model not found") {
					t.Skipf("provider %s is not a vision model: %s", tc.name, msg)
				}
				t.Fatalf("classify: %v", err)
			}
			if !v.NSFW {
				t.Fatalf("expected nsfw=true, got nsfw=%v confidence=%d reason=%q", v.NSFW, v.Confidence, v.Reason)
			}
			if !v.ShouldBlock(50) {
				t.Fatalf("expected block, confidence=%d", v.Confidence)
			}
		})
		if !t.Failed() {
			ran++
		}
	}
	_ = ran
}

func TestLiveVisionLatency(t *testing.T) {
	loadAIDotEnv(t)
	img, reqs := liveVisionRequests(t)
	_ = img
	const rounds = 3
	for _, req := range reqs {
		t.Run(req.Provider, func(t *testing.T) {
			var times []time.Duration
			for i := 0; i < rounds; i++ {
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				start := time.Now()
				v, err := ClassifyImage(ctx, req)
				elapsed := time.Since(start)
				cancel()
				if err != nil {
					t.Fatalf("round %d: %v", i+1, err)
				}
				if !v.NSFW {
					t.Fatalf("round %d: expected nsfw, confidence=%d reason=%q", i+1, v.Confidence, v.Reason)
				}
				times = append(times, elapsed)
				kind := "warm"
				if i == 0 {
					kind = "cold"
				}
				t.Logf("%s round %d %s %s nsfw=%v confidence=%d", req.Provider, i+1, kind, elapsed.Round(time.Millisecond), v.NSFW, v.Confidence)
			}
			var sum time.Duration
			min, max := times[0], times[0]
			for _, d := range times {
				sum += d
				if d < min {
					min = d
				}
				if d > max {
					max = d
				}
			}
			avg := sum / time.Duration(len(times))
			t.Logf("%s summary n=%d min=%s avg=%s max=%s (jpeg %d bytes)",
				req.Provider, len(times), min.Round(time.Millisecond), avg.Round(time.Millisecond), max.Round(time.Millisecond), len(req.Image))
		})
	}
}

func liveVisionRequests(t *testing.T) ([]byte, []VisionRequest) {
	t.Helper()
	imgPath := strings.TrimSpace(os.Getenv("GS_AI_TEST_IMAGE"))
	if imgPath == "" {
		imgPath = "/home/jbarwick/Development/33091992_018_4cb3.jpg"
	}
	img, err := os.ReadFile(imgPath)
	if err != nil {
		t.Skipf("test image not available: %v", err)
	}
	specs := []struct {
		provider, keyEnv, urlEnv, modelEnv string
	}{
		{AIModeGrok, "GS_AI_GROK_API_KEY", "", "GS_AI_GROK_MODEL"},
		{AIModeChatGPT, "GS_AI_OPENAI_API_KEY", "", "GS_AI_OPENAI_MODEL"},
		{AIModeLocal, "", "GS_AI_OLLAMA_URL", "GS_AI_OLLAMA_MODEL"},
	}
	var reqs []VisionRequest
	for _, s := range specs {
		req := VisionRequest{
			Provider:    s.provider,
			APIKey:      os.Getenv(s.keyEnv),
			BaseURL:     os.Getenv(s.urlEnv),
			Model:       os.Getenv(s.modelEnv),
			ContentType: "image/jpeg",
			Image:       img,
		}
		if s.provider == AIModeLocal {
			if EnvLooksUnset(req.BaseURL) {
				continue
			}
		} else if EnvLooksUnset(req.APIKey) {
			continue
		}
		reqs = append(reqs, req)
	}
	if len(reqs) == 0 {
		t.Skip("no AI providers configured")
	}
	return img, reqs
}

func loadAIDotEnv(t *testing.T) {
	t.Helper()
	candidates := []string{".env", "../.env", "../../.env"}
	if wd, err := os.Getwd(); err == nil {
		dir := wd
		for i := 0; i < 6; i++ {
			candidates = append(candidates, filepath.Join(dir, ".env"))
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	seen := map[string]bool{}
	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil || seen[abs] {
			continue
		}
		seen[abs] = true
		applyDotEnvFile(abs)
	}
}

func applyDotEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		if k == "" {
			continue
		}
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}
