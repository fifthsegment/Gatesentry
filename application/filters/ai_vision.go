package gatesentry2filters

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultGrokVisionModel   = "grok-4.5"
	defaultOpenAIVisionModel = "gpt-4o-mini"
	defaultOllamaVisionModel = "llava"
	visionClassifyTimeout    = 60 * time.Second
	visionMaxImageBytes      = 8 << 20
)

const visionClassifyPrompt = `Classify this image for a household content filter.
Reply with one JSON object only:
{"nsfw": true|false, "confidence": 0-100, "reason": "short"}
nsfw=true for pornography, sexual nudity, explicit sexual activity, or clearly sexualized poses meant as adult content.
nsfw=false for non-sexual clothing, fashion, or ordinary photos of people.`

// ImageVerdict is a model classification of one image. Do not log Raw.
type ImageVerdict struct {
	NSFW       bool   `json:"nsfw"`
	Confidence int    `json:"confidence"`
	Reason     string `json:"reason"`
	Raw        string `json:"-"`
}

// ShouldBlock reports whether the verdict is inappropriate at minConfidence (0-100).
func (v ImageVerdict) ShouldBlock(minConfidence int) bool {
	if minConfidence < 0 {
		minConfidence = 0
	}
	return v.NSFW && v.Confidence >= minConfidence
}

// VisionRequest is one classification call.
type VisionRequest struct {
	Provider    string // grok, chatgpt, local
	APIKey      string
	BaseURL     string // required for local (Ollama); ignored for grok/chatgpt
	Model       string
	ContentType string
	Image       []byte
	Client      *http.Client
}

type chatCompletionRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	MaxTokens      int           `json:"max_tokens,omitempty"`
	ResponseFormat *chatFormat   `json:"response_format,omitempty"`
}

type chatFormat struct {
	Type string `json:"type"`
}

type chatMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// ClassifyImage sends image bytes to Grok, ChatGPT, or Ollama and parses the JSON verdict.
func ClassifyImage(ctx context.Context, req VisionRequest) (ImageVerdict, error) {
	if len(req.Image) == 0 {
		return ImageVerdict{}, fmt.Errorf("empty image")
	}
	if len(req.Image) > visionMaxImageBytes {
		return ImageVerdict{}, fmt.Errorf("image too large (%d bytes)", len(req.Image))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	provider := strings.ToLower(strings.TrimSpace(req.Provider))
	endpoint, model, headers, err := visionEndpoint(provider, req)
	if err != nil {
		return ImageVerdict{}, err
	}
	ct := req.ContentType
	if ct == "" {
		ct = "image/jpeg"
	}
	var body []byte
	if provider == AIModeLocal {
		body, err = buildOllamaChatBody(model, req.Image)
	} else {
		body, err = buildVisionChatBody(model, ct, req.Image, provider == AIModeChatGPT)
	}
	if err != nil {
		return ImageVerdict{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ImageVerdict{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "gatesentry-ai-vision")
	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	client := req.Client
	if client == nil {
		client = newVisionHTTPClient()
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return ImageVerdict{}, fmt.Errorf("vision request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return ImageVerdict{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ImageVerdict{}, fmt.Errorf("vision HTTP %d: %s", resp.StatusCode, clipVisionError(raw))
	}
	text, err := visionReplyText(provider, raw)
	if err != nil {
		return ImageVerdict{}, err
	}
	return ParseImageVerdictJSON(text)
}

func visionEndpoint(provider string, req VisionRequest) (endpoint, model string, headers map[string]string, err error) {
	headers = map[string]string{}
	switch provider {
	case AIModeGrok:
		model = strings.TrimSpace(req.Model)
		if model == "" {
			model = defaultGrokVisionModel
		}
		if strings.TrimSpace(req.APIKey) == "" {
			return "", "", nil, fmt.Errorf("grok API key is empty")
		}
		headers["Authorization"] = "Bearer " + strings.TrimSpace(req.APIKey)
		return "https://api.x.ai/v1/chat/completions", model, headers, nil
	case AIModeChatGPT:
		model = strings.TrimSpace(req.Model)
		if model == "" {
			model = defaultOpenAIVisionModel
		}
		if strings.TrimSpace(req.APIKey) == "" {
			return "", "", nil, fmt.Errorf("openai API key is empty")
		}
		headers["Authorization"] = "Bearer " + strings.TrimSpace(req.APIKey)
		return "https://api.openai.com/v1/chat/completions", model, headers, nil
	case AIModeLocal:
		base := strings.TrimRight(strings.TrimSpace(req.BaseURL), "/")
		if base == "" {
			return "", "", nil, fmt.Errorf("ollama URL is empty")
		}
		if _, err := url.ParseRequestURI(base); err != nil {
			return "", "", nil, fmt.Errorf("ollama URL is invalid")
		}
		model = strings.TrimSpace(req.Model)
		if model == "" {
			model = defaultOllamaVisionModel
		}
		if k := strings.TrimSpace(req.APIKey); k != "" {
			headers["Authorization"] = "Bearer " + k
		}
		return base + "/api/chat", model, headers, nil
	default:
		return "", "", nil, fmt.Errorf("unknown vision provider %q", provider)
	}
}

func buildVisionChatBody(model, contentType string, image []byte, jsonObjectFormat bool) ([]byte, error) {
	dataURI := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(image)
	content, err := json.Marshal([]map[string]interface{}{
		{"type": "text", "text": visionClassifyPrompt},
		{"type": "image_url", "image_url": map[string]string{"url": dataURI}},
	})
	if err != nil {
		return nil, err
	}
	req := chatCompletionRequest{
		Model:     model,
		MaxTokens: 200,
		Messages: []chatMessage{
			{Role: "user", Content: content},
		},
	}
	if jsonObjectFormat {
		req.ResponseFormat = &chatFormat{Type: "json_object"}
	}
	return json.Marshal(req)
}

func buildOllamaChatBody(model string, image []byte) ([]byte, error) {
	msg := map[string]interface{}{
		"role":    "user",
		"content": visionClassifyPrompt,
		"images":  []string{base64.StdEncoding.EncodeToString(image)},
	}
	return json.Marshal(map[string]interface{}{
		"model":    model,
		"stream":   false,
		"format":   "json",
		"messages": []map[string]interface{}{msg},
	})
}

func visionReplyText(provider string, raw []byte) (string, error) {
	if provider == AIModeLocal {
		var ollama struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Error string `json:"error"`
		}
		if err := json.Unmarshal(raw, &ollama); err != nil {
			return "", fmt.Errorf("ollama response is not JSON")
		}
		if ollama.Error != "" {
			return "", fmt.Errorf("ollama: %s", ollama.Error)
		}
		return ollama.Message.Content, nil
	}
	var parsed chatCompletionResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("vision response is not chat JSON")
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("vision API: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("vision API returned no choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func clipVisionError(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 240 {
		s = s[:240] + "…"
	}
	return s
}

// ParseImageVerdictJSON extracts {"nsfw","confidence","reason"} from a model reply.
func ParseImageVerdictJSON(text string) (ImageVerdict, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return ImageVerdict{}, fmt.Errorf("empty model reply")
	}
	text = strings.ReplaceAll(text, "```json", "")
	text = strings.ReplaceAll(text, "```", "")
	candidate := extractNSFWObject(text)
	if candidate == "" {
		return ImageVerdict{}, fmt.Errorf("model reply is not verdict JSON")
	}
	var v ImageVerdict
	if err := json.Unmarshal([]byte(candidate), &v); err != nil {
		return ImageVerdict{}, fmt.Errorf("model reply is not verdict JSON")
	}
	if v.Confidence < 0 {
		v.Confidence = 0
	}
	if v.Confidence > 100 {
		v.Confidence = 100
	}
	v.Raw = candidate
	return v, nil
}

func extractNSFWObject(text string) string {
	for i, r := range text {
		if r != '{' {
			continue
		}
		depth := 0
		inStr := false
		esc := false
		for j := i; j < len(text); j++ {
			c := text[j]
			if inStr {
				if esc {
					esc = false
					continue
				}
				if c == '\\' {
					esc = true
					continue
				}
				if c == '"' {
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					chunk := text[i : j+1]
					if strings.Contains(strings.ToLower(chunk), `"nsfw"`) {
						return chunk
					}
					break
				}
			}
		}
	}
	if i := strings.Index(text, "{"); i >= 0 {
		if j := strings.LastIndex(text, "}"); j > i {
			return text[i : j+1]
		}
	}
	return ""
}

func newVisionHTTPClient() *http.Client {
	timeout := visionClassifyTimeout
	dialer := &net.Dialer{Timeout: 15 * time.Second, KeepAlive: -1}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return nil, nil
			},
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   15 * time.Second,
			ResponseHeaderTimeout: timeout,
			DisableKeepAlives:     true,
			ForceAttemptHTTP2:     false,
		},
	}
}
