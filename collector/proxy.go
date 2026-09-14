package collector

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"tokenmonitor/config"
)

var bufPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, 32*1024))
	},
}

type ProxyServer struct {
	cfg        *config.Config
	buffer     *AsyncBuffer
	reverse    *httputil.ReverseProxy
	upstreamURL *url.URL
}

func NewProxyServer(cfg *config.Config, buffer *AsyncBuffer) (*ProxyServer, error) {
	u, err := url.Parse(cfg.Proxy.UpstreamTarget)
	if err != nil {
		return nil, fmt.Errorf("lỗi parse upstream URL: %w", err)
	}

	proxy := &ProxyServer{
		cfg:         cfg,
		buffer:      buffer,
		upstreamURL: u,
	}

	director := func(req *http.Request) {
		req.URL.Scheme = u.Scheme
		req.URL.Host = u.Host
		req.Host = u.Host
	}

	rev := &httputil.ReverseProxy{
		Director: director,
		ModifyResponse: proxy.modifyResponse,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("[ERROR] Proxy kết nối Upstream lỗi: %v", err)
			http.Error(w, fmt.Sprintf(`{"error":"Proxy Upstream Error: %v"}`, err), http.StatusBadGateway)
		},
	}

	proxy.reverse = rev
	return proxy, nil
}

func (p *ProxyServer) modifyResponse(resp *http.Response) error {
	// Chỉ phân tích JSON response từ các cuộc gọi generateContent
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return nil
	}

	// Đọc toàn bộ body để bóc tách usageMetadata
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()

	// Trả lại body nguyên vẹn cho downstream client
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// Chạy async parsing metadata để không block luồng client
	go p.parseAndRecordMetadata(resp.Request.URL.Path, resp.StatusCode, bodyBytes)

	return nil
}

func (p *ProxyServer) parseAndRecordMetadata(reqPath string, statusCode int, body []byte) {
	var geminiResp GeminiResponseWrapper
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return
	}

	if geminiResp.UsageMetadata == nil {
		return
	}

	meta := geminiResp.UsageMetadata
	model := "Gemini 3.8 Flash (High)"
	if geminiResp.ModelVersion != "" {
		model = NormalizeModelName(geminiResp.ModelVersion)
	} else if strings.Contains(strings.ToLower(reqPath), "ultra") {
		model = "Gemini Ultra"
	} else if strings.Contains(strings.ToLower(reqPath), "pro") {
		model = "Gemini 3.1 Pro (High)"
	} else if strings.Contains(strings.ToLower(reqPath), "3.7") {
		model = "Gemini 3.7 Flash Medium"
	} else if strings.Contains(strings.ToLower(reqPath), "3.6") {
		model = "Gemini 3.6 Flash Medium"
	} else if strings.Contains(strings.ToLower(reqPath), "flash") {
		model = "Gemini 3.8 Flash (High)"
	}

	thinkingTokens := meta.GetThinkingTokens()

	event := &TokenUsageEvent{
		AccountID:      1, // Mặc định tài khoản chính
		Timestamp:      time.Now(),
		ModelName:      model,
		PromptTokens:   meta.PromptTokenCount,
		OutputTokens:   meta.CandidatesTokenCount,
		ThinkingTokens: thinkingTokens,
		CachedTokens:   meta.CachedContentTokenCount,
		TotalTokens:    meta.TotalTokenCount,
		LatencyMs:      1200, // Ước lượng hoặc đo từ request context
		StatusCode:     statusCode,
		RequestType:    "INTERACTIVE",
	}

	p.buffer.Push(event)
}

func (p *ProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS Headers cho phép truy cập linh hoạt
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	p.reverse.ServeHTTP(w, r)
}
