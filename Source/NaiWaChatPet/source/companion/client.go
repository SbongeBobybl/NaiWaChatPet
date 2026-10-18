package companion

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const deepSeekEndpoint = "https://api.deepseek.com/chat/completions"
const maxResponseBytes = 8 << 20

var modelPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,95}$`)
var httpClient = &http.Client{Timeout: 150 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("model endpoint redirects are disabled") }}

type Message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}
type RequestOptions struct {
	JSONObject        bool
	ThinkingParameter string
	RequestID, Model  string
	Stream, Thinking  bool
	MaxTokens         int
}
type chatRequest struct {
	ResponseFormat map[string]string `json:"response_format,omitempty"`
	Model          string            `json:"model"`
	Messages       []Message         `json:"messages"`
	Stream         bool              `json:"stream"`
	MaxTokens      int               `json:"max_tokens"`
	Thinking       map[string]string `json:"thinking,omitempty"`
}
type Result struct {
	Text, FinishReason   string
	ReasoningSeen        bool
	HTTPStatus, Attempts int
}

// Diagnostics deliberately contain no prompt, reply, key, URL query, or image.
type Diagnostic struct {
	RequestID  string `json:"request_id"`
	Model      string `json:"model"`
	HTTPStatus int    `json:"http_status"`
	DurationMS int64  `json:"duration_ms"`
	Attempt    int    `json:"attempt"`
	EndReason  string `json:"end_reason"`
}
type Client struct {
	HTTP       *http.Client
	Endpoint   string
	Attempts   int
	Diagnostic func(Diagnostic)
	Wait       func(context.Context, time.Duration) error
}
type ReplyError struct {
	Code, Message string
	HTTPStatus    int
	Retryable     bool
	Partial       bool
	RetryAfter    time.Duration
}

func (e *ReplyError) Error() string { return e.Message }
func replyError(code string, status int, partial bool) *ReplyError {
	messages := map[string]string{
		"empty":       "模型没有返回正文，请重试或切换到非流式诊断模式。",
		"truncated":   "回复达到生成长度上限，尚未完成；可要求继续或提高回复长度。",
		"interrupted": "网络连接在回复完成前中断；已显示的部分不会自动重发。",
		"protocol":    "模型返回了无法解析的响应，请检查模型兼容性。",
		"cancelled":   "本次请求已取消。",
		"timeout":     "等待模型超时，请稍后重试。",
		"network":     "无法连接模型服务，请检查网络连接。",
		"invalid":     "模型设置或请求格式有误（400）。",
		"auth":        "API Key 验证失败（401），请在设置中检查。",
		"balance":     "DeepSeek 账户余额不足（402）。",
		"rate_limit":  "请求过于频繁（429），请稍后重试。",
		"server":      "模型服务暂时不可用（5xx），请稍后重试。",
		"filtered":    "模型未完成这次回复，请调整问题后重试。",
		"unsupported": "这个模型暂不支持所选功能，请检查模型能力设置。",
	}
	m := messages[code]
	if m == "" {
		m = "模型请求失败，请在设置中查看诊断。"
	}
	return &ReplyError{Code: code, Message: m, HTTPStatus: status, Partial: partial, Retryable: code == "network" || code == "rate_limit" || code == "server"}
}
func classifyTransport(ctx context.Context, err error, partial bool) *ReplyError {
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return replyError("cancelled", 0, partial)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return replyError("timeout", 0, partial)
	}
	var n net.Error
	if errors.As(err, &n) && n.Timeout() {
		return replyError("timeout", 0, partial)
	}
	return replyError("network", 0, partial)
}
func waitRetry(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) Complete(ctx context.Context, key string, o RequestOptions, messages []Message, onDelta func(string)) (Result, error) {
	if ctx == nil {
		return Result{}, replyError("invalid", 0, false)
	}
	if len(key) < 8 || len(key) > 512 || strings.ContainsAny(key, "\r\n\t ") {
		return Result{}, replyError("auth", 401, false)
	}
	if !modelPattern.MatchString(o.Model) || strings.Contains(o.Model, key) {
		return Result{}, replyError("invalid", 400, false)
	}
	if o.MaxTokens < 128 || o.MaxTokens > 32768 {
		o.MaxTokens = 2048
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = deepSeekEndpoint
	}
	thinking := "disabled"
	if o.Thinking {
		thinking = "enabled"
	}
	payload := chatRequest{Model: o.Model, Messages: messages, Stream: o.Stream, MaxTokens: o.MaxTokens}
	if o.JSONObject {
		payload.Stream = false
		payload.ResponseFormat = map[string]string{"type": "json_object"}
	}
	// Compatible custom IDs can opt out of the vendor extension in model config.
	if o.ThinkingParameter != "omit" && (o.ThinkingParameter == "deepseek" || strings.Contains(endpoint, "api.deepseek.com/") || o.Model == "deepseek-flash" || o.Model == "deepseek-v4-pro") {
		payload.Thinking = map[string]string{"type": thinking}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return Result{}, replyError("invalid", 400, false)
	}
	client := c.HTTP
	if client == nil {
		client = httpClient
	}
	attempts := c.Attempts
	if attempts < 1 {
		attempts = 2
	}
	if attempts > 3 {
		attempts = 3
	}
	waiter := c.Wait
	if waiter == nil {
		waiter = waitRetry
	}
	var result Result
	for attempt := 1; attempt <= attempts; attempt++ {
		started := time.Now()
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
		if e != nil {
			return Result{}, replyError("invalid", 0, false)
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		resp, e := client.Do(req)
		if e != nil {
			err = classifyTransport(ctx, e, false)
			result = Result{Attempts: attempt}
		} else {
			result, err = consumeResponse(ctx, resp, onDelta)
			result.Attempts = attempt
			_ = resp.Body.Close()
		}
		reason := result.FinishReason
		if err != nil {
			var re *ReplyError
			if errors.As(err, &re) {
				reason = re.Code
			} else {
				reason = "error"
			}
		}
		if c.Diagnostic != nil {
			c.Diagnostic(Diagnostic{o.RequestID, o.Model, result.HTTPStatus, time.Since(started).Milliseconds(), attempt, reason})
		}
		if err == nil {
			return result, nil
		}
		var re *ReplyError
		// Any generated text, including hidden reasoning, prevents automatic replay.
		if !errors.As(err, &re) || !re.Retryable || re.Partial || result.ReasoningSeen || result.Text != "" || attempt == attempts {
			return result, err
		}
		delay := time.Duration(attempt) * 200 * time.Millisecond
		if re.RetryAfter > delay {
			delay = re.RetryAfter
		}
		if delay > 2*time.Second {
			delay = 2 * time.Second
		}
		if e = waiter(ctx, delay); e != nil {
			return result, classifyTransport(ctx, e, false)
		}
	}
	return result, err
}

type wireChoice struct {
	Index        int     `json:"index"`
	FinishReason *string `json:"finish_reason"`
	Delta        struct {
		Content   *string `json:"content"`
		Reasoning *string `json:"reasoning_content"`
	} `json:"delta"`
	Message struct {
		Content   *string `json:"content"`
		Reasoning *string `json:"reasoning_content"`
	} `json:"message"`
}
type wireReply struct {
	Choices []wireChoice    `json:"choices"`
	Error   json.RawMessage `json:"error"`
}

func consumeResponse(ctx context.Context, resp *http.Response, onDelta func(string)) (Result, error) {
	result := Result{HTTPStatus: resp.StatusCode}
	if resp.StatusCode != http.StatusOK {
		// Never propagate vendor error bodies: they may echo private prompt details.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
		code := "server"
		switch resp.StatusCode {
		case 400:
			code = "invalid"
		case 401, 403:
			code = "auth"
		case 402:
			code = "balance"
		case 429:
			code = "rate_limit"
		default:
			if resp.StatusCode < 500 {
				code = "unsupported"
			}
		}
		e := replyError(code, resp.StatusCode, false)
		if sec, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && sec > 0 {
			e.RetryAfter = time.Duration(min(sec, 2)) * time.Second
		}
		return result, e
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		return readSSE(ctx, resp.Body, result, onDelta)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return result, classifyTransport(ctx, err, false)
	}
	if len(b) > maxResponseBytes {
		return result, replyError("protocol", 200, false)
	}
	var w wireReply
	if json.Unmarshal(b, &w) != nil || len(w.Error) > 0 && string(w.Error) != "null" {
		return result, replyError("protocol", 200, false)
	}
	for _, ch := range w.Choices {
		if ch.Index != 0 {
			continue
		}
		if ch.Message.Content != nil {
			result.Text = *ch.Message.Content
		}
		result.ReasoningSeen = ch.Message.Reasoning != nil && *ch.Message.Reasoning != ""
		if ch.FinishReason != nil {
			result.FinishReason = *ch.FinishReason
		}
		break
	}
	result.Text = strings.TrimSpace(result.Text)
	if result.Text != "" && onDelta != nil {
		onDelta(result.Text)
	}
	return validateCompletion(result, true)
}
func validateCompletion(r Result, done bool) (Result, error) {
	if !done {
		return r, replyError("interrupted", r.HTTPStatus, r.Text != "" || r.ReasoningSeen)
	}
	switch r.FinishReason {
	case "stop":
	case "length":
		return r, replyError("truncated", r.HTTPStatus, r.Text != "")
	case "content_filter", "tool_calls":
		return r, replyError("filtered", r.HTTPStatus, r.Text != "")
	default:
		return r, replyError("interrupted", r.HTTPStatus, r.Text != "" || r.ReasoningSeen)
	}
	r.Text = strings.TrimSpace(r.Text)
	if r.Text == "" {
		return r, replyError("empty", r.HTTPStatus, false)
	}
	return r, nil
}
func readSSE(ctx context.Context, body io.Reader, result Result, onDelta func(string)) (Result, error) {
	limited := &io.LimitedReader{R: body, N: maxResponseBytes + 1}
	reader := bufio.NewReader(limited)
	var data []string
	var eventBytes int
	done := false
	process := func() error {
		if len(data) == 0 {
			return nil
		}
		event := strings.Join(data, "\n")
		data = nil
		eventBytes = 0
		if strings.TrimSpace(event) == "[DONE]" {
			done = true
			return nil
		}
		var w wireReply
		if json.Unmarshal([]byte(event), &w) != nil || len(w.Error) > 0 && string(w.Error) != "null" {
			return replyError("protocol", 200, result.Text != "" || result.ReasoningSeen)
		}
		for _, ch := range w.Choices {
			if ch.Index != 0 {
				continue
			}
			if ch.Delta.Reasoning != nil && *ch.Delta.Reasoning != "" {
				result.ReasoningSeen = true
			}
			if ch.Delta.Content != nil && *ch.Delta.Content != "" {
				delta := *ch.Delta.Content
				result.Text += delta
				if onDelta != nil {
					onDelta(delta)
				}
			}
			if ch.FinishReason != nil && *ch.FinishReason != "" {
				result.FinishReason = *ch.FinishReason
			}
			break
		}
		return nil
	}
	for !done {
		if ctx.Err() != nil {
			return result, classifyTransport(ctx, ctx.Err(), result.Text != "" || result.ReasoningSeen)
		}
		line, err := reader.ReadString('\n')
		if limited.N <= 0 {
			return result, replyError("protocol", 200, result.Text != "")
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if line == "" {
			if e := process(); e != nil {
				return result, e
			}
		} else if strings.HasPrefix(line, "data:") {
			v := strings.TrimPrefix(line, "data:")
			v = strings.TrimPrefix(v, " ")
			eventBytes += len(v)
			if eventBytes > 2<<20 {
				return result, replyError("protocol", 200, result.Text != "")
			}
			data = append(data, v)
		}
		if err != nil {
			if e := process(); e != nil {
				return result, e
			}
			if done {
				break
			}
			if ctx.Err() != nil {
				return result, classifyTransport(ctx, ctx.Err(), result.Text != "" || result.ReasoningSeen)
			}
			if err != io.EOF {
				return result, classifyTransport(ctx, err, result.Text != "" || result.ReasoningSeen)
			}
			return result, replyError("interrupted", 200, result.Text != "" || result.ReasoningSeen)
		}
	}
	return validateCompletion(result, done)
}

// Compatibility wrapper for the existing synchronous HTTP route; the V12
// service uses Complete directly for incremental native-window updates.
func callDeepSeek(ctx context.Context, key, model string, messages []Message, maxTokens int) (string, error) {
	c := Client{HTTP: httpClient, Attempts: 2}
	r, e := c.Complete(ctx, key, RequestOptions{Model: model, MaxTokens: maxTokens}, messages, nil)
	return r.Text, e
}
func imageContent(text string, jpeg []byte) []map[string]any {
	return []map[string]any{{"type": "text", "text": text}, {"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(jpeg)}}}
}
