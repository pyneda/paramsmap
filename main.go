package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
)

var logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
var numBaselines = 3

type Config struct {
	Timeout             int
	IgnoreCertErrors    bool
	Concurrency         int
	Headers             []string
	SimilarityThreshold float64
	ReportPath          string
	httpClient          *http.Client
	totalRequests       atomic.Int64
}

type headerFlags []string

func (h *headerFlags) String() string { return strings.Join(*h, ", ") }
func (h *headerFlags) Set(value string) error {
	*h = append(*h, value)
	return nil
}

func main() {
	var requestURL, method, postData, contentType, wordlist, reportPath string
	var chunkSize, timeout, concurrency int
	var ignoreCertErrors bool
	var similarityThreshold float64
	var headers headerFlags

	flag.StringVar(&requestURL, "url", "", "The URL to make the request to")
	flag.StringVar(&method, "method", "GET", "HTTP method to use")
	flag.StringVar(&postData, "data", "", "Optional body data")
	flag.StringVar(&contentType, "type", "form", "Content type: form, json, xml")
	flag.StringVar(&wordlist, "wordlist", "wordlist.txt", "Path to the wordlist file")
	flag.StringVar(&reportPath, "report", "report.json", "Path to the output report file")
	flag.IntVar(&chunkSize, "chunk-size", 1000, "Number of parameters to send in each request")
	flag.IntVar(&timeout, "timeout", 10, "Request timeout in seconds")
	flag.BoolVar(&ignoreCertErrors, "ignore-cert", false, "Ignore SSL certificate errors")
	flag.IntVar(&concurrency, "concurrency", 10, "Maximum number of concurrent requests")
	flag.Float64Var(&similarityThreshold, "similarity", 0.9, "Similarity threshold for response comparison (0.0-1.0)")
	flag.Var(&headers, "H", "Custom header (can be specified multiple times, e.g. -H 'Cookie: foo=bar')")

	flag.Parse()

	if requestURL == "" {
		logger.Error("URL is required")
		return
	}

	if concurrency < 1 {
		logger.Error("Concurrency must be at least 1")
		return
	}

	cfg := &Config{
		Timeout:             timeout,
		IgnoreCertErrors:    ignoreCertErrors,
		Concurrency:         concurrency,
		Headers:             headers,
		SimilarityThreshold: similarityThreshold,
		ReportPath:          reportPath,
		httpClient:          createHTTPClient(timeout, ignoreCertErrors),
	}

	params := loadWordlist(wordlist)
	logger.Info("Loaded parameters from wordlist", "count", len(params))

	request := Request{
		URL:         requestURL,
		Method:      method,
		Data:        postData,
		ContentType: contentType,
	}

	results := DiscoverParams(cfg, request, params, chunkSize)
	logger.Info("Total requests made", "count", cfg.totalRequests.Load())
	logger.Info("Valid parameters found", "count", len(results.Params), "valid", results.Params)
	logger.Info("Form parameters found", "count", len(results.FormParams), "parameters", results.FormParams)
	if cfg.ReportPath != "" {
		saveReport(cfg.ReportPath, results)
	}
}

type Request struct {
	URL         string `json:"url"`
	Method      string `json:"method"`
	Data        string `json:"data"`
	ContentType string `json:"content_type"`
}

type Results struct {
	Params        []string `json:"params"`
	FormParams    []string `json:"form_params"`
	TotalRequests int      `json:"total_requests"`
	Aborted       bool     `json:"aborted"`
	AbortReason   string   `json:"abort_reason"`
	Request       Request  `json:"request"`
}

type ResponseData struct {
	Body        []byte
	BodyHash    [32]byte
	StatusCode  int
	Reflections int
}

type InitialResponses struct {
	Responses     []ResponseData
	SameBody      bool
	AreConsistent bool
}

func DiscoverParams(cfg *Config, request Request, params []string, chunkSize int) Results {
	initialResponses := makeInitialRequests(cfg, request)

	if !initialResponses.AreConsistent {
		logger.Warn("Baseline responses differ significantly. The page appears to be too dynamic. Scanning will be skipped.")
		return Results{
			Params:        []string{},
			FormParams:    []string{},
			Aborted:       true,
			AbortReason:   "Baseline responses differ significantly",
			TotalRequests: int(cfg.totalRequests.Load()),
			Request:       request,
		}
	}

	formParams := extractFormParams(initialResponses.Responses[0].Body)
	logger.Info("Extracted form parameters", "count", len(formParams), "parameters", formParams)

	// Discover params from wordlist (form params are NOT appended to avoid false positives)
	validParams := discoverValidParams(cfg, request, params, initialResponses, chunkSize)

	// Test form params separately, ignoring reflections to avoid false positives
	validFormParams := testFormParams(cfg, request, formParams, initialResponses)

	// Merge form param results, deduplicating
	paramSet := make(map[string]bool)
	for _, p := range validParams {
		paramSet[p] = true
	}
	for _, p := range validFormParams {
		if !paramSet[p] {
			paramSet[p] = true
			validParams = append(validParams, p)
		}
	}

	return Results{
		Params:        validParams,
		FormParams:    formParams,
		TotalRequests: int(cfg.totalRequests.Load()),
		Request:       request,
	}
}

// testFormParams tests each form parameter individually, ignoring reflection
// differences to avoid false positives from parameters that are already present
// in the baseline HTML.
func testFormParams(cfg *Config, request Request, formParams []string, initialResponses InitialResponses) []string {
	var validParams []string
	for _, param := range formParams {
		params := url.Values{}
		params.Set(param, randomString(8))
		response := makeRequest(cfg, request, params)
		if responseChangedIgnoringReflections(initialResponses.Responses, response, initialResponses.SameBody, cfg.SimilarityThreshold) {
			validParams = append(validParams, param)
		}
	}
	return validParams
}

func discoverValidParams(cfg *Config, request Request, params []string, initialResponses InitialResponses, chunkSize int) []string {
	parts := chunkParams(params, chunkSize)
	validParts := filterParts(cfg, request, parts, initialResponses)

	paramSet := make(map[string]bool)
	var validParams []string
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, cfg.Concurrency)

	for _, part := range validParts {
		wg.Add(1)
		sem <- struct{}{}
		go func(part []string) {
			defer wg.Done()
			defer func() { <-sem }()
			for _, param := range recursiveFilter(cfg, request, part, initialResponses) {
				mu.Lock()
				if !paramSet[param] {
					paramSet[param] = true
					validParams = append(validParams, param)
					logger.Info("Valid parameter discovered", "parameter", param)
				}
				mu.Unlock()
			}
		}(part)
	}

	wg.Wait()
	return validParams
}

func filterParts(cfg *Config, request Request, parts [][]string, initialResponses InitialResponses) [][]string {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var validParts [][]string
	sem := make(chan struct{}, cfg.Concurrency)

	for _, part := range parts {
		wg.Add(1)
		sem <- struct{}{}
		go func(part []string) {
			defer wg.Done()
			defer func() { <-sem }()
			params := generateParams(part)
			response := makeRequest(cfg, request, params)

			if responseChanged(initialResponses.Responses, response, initialResponses.SameBody, cfg.SimilarityThreshold) {
				mu.Lock()
				validParts = append(validParts, part)
				mu.Unlock()
			}
		}(part)
	}
	wg.Wait()
	return validParts
}

func recursiveFilter(cfg *Config, request Request, params []string, initialResponses InitialResponses) []string {
	if len(params) == 1 {
		return params
	}
	mid := len(params) / 2
	left := params[:mid]
	right := params[mid:]

	leftParams := generateParams(left)
	rightParams := generateParams(right)

	leftResponse := makeRequest(cfg, request, leftParams)
	rightResponse := makeRequest(cfg, request, rightParams)

	var validParams []string
	if responseChanged(initialResponses.Responses, leftResponse, initialResponses.SameBody, cfg.SimilarityThreshold) {
		validParams = append(validParams, recursiveFilter(cfg, request, left, initialResponses)...)
	}
	if responseChanged(initialResponses.Responses, rightResponse, initialResponses.SameBody, cfg.SimilarityThreshold) {
		validParams = append(validParams, recursiveFilter(cfg, request, right, initialResponses)...)
	}
	return validParams
}

func makeInitialRequests(cfg *Config, request Request) InitialResponses {
	var baselineResponses []ResponseData
	for i := 0; i < numBaselines; i++ {
		resp := makeRequest(cfg, request, url.Values{})
		baselineResponses = append(baselineResponses, resp)
	}

	return InitialResponses{
		Responses: baselineResponses,
		SameBody:  baselineResponsesAreConsistent(baselineResponses, responsesAreEqual),
		AreConsistent: baselineResponsesAreConsistent(baselineResponses, func(a, b ResponseData) bool {
			return responsesAreSimilar(a, b, cfg.SimilarityThreshold)
		}),
	}
}

func makeRequest(cfg *Config, request Request, params url.Values) ResponseData {
	cfg.totalRequests.Add(1)

	parsedURL, err := url.Parse(request.URL)
	if err != nil {
		logger.Error("Failed to parse request URL", "error", err)
		return ResponseData{}
	}

	existingParams := parsedURL.Query()
	for key, values := range params {
		for _, value := range values {
			existingParams.Add(key, value)
		}
	}
	parsedURL.RawQuery = existingParams.Encode()
	requestURL := parsedURL.String()

	var req *http.Request
	if request.Method == "GET" {
		req, err = http.NewRequest(request.Method, requestURL, nil)
	} else if request.ContentType == "json" {
		req, err = http.NewRequest(request.Method, requestURL, bytes.NewBufferString(request.Data))
	} else if request.ContentType == "xml" {
		req, err = http.NewRequest(request.Method, requestURL, bytes.NewBufferString(request.Data))
	} else {
		req, err = http.NewRequest(request.Method, requestURL, strings.NewReader(params.Encode()))
	}
	if err != nil {
		logger.Error("Failed to create request", "error", err)
		return ResponseData{}
	}

	// Set content type for non-GET requests
	if request.Method != "GET" {
		switch request.ContentType {
		case "json":
			req.Header.Set("Content-Type", "application/json")
		case "xml":
			req.Header.Set("Content-Type", "application/xml")
		default:
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}

	req.Header.Set("User-Agent", randomUserAgent())

	// Apply custom headers
	for _, h := range cfg.Headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}

	resp, err := cfg.httpClient.Do(req)
	if err != nil {
		logger.Error("Failed to make request", "error", err)
		return ResponseData{}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response body", "error", err)
		return ResponseData{}
	}

	reflections := countReflections(params, body)
	hash := sha256.Sum256(body)
	return ResponseData{
		Body:        body,
		BodyHash:    hash,
		StatusCode:  resp.StatusCode,
		Reflections: reflections,
	}
}

func saveReport(reportPath string, results Results) {
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		logger.Error("Error marshalling results to JSON", slog.String("error", err.Error()))
		return
	}

	file, err := os.Create(reportPath)
	if err != nil {
		logger.Error("Error creating/opening report file", slog.String("error", err.Error()), slog.String("path", reportPath))
		return
	}
	defer file.Close()

	_, err = file.Write(jsonData)
	if err != nil {
		logger.Error("Error writing JSON to file", slog.String("error", err.Error()), slog.String("path", reportPath))
		return
	}

	logger.Info("Report saved successfully", slog.String("path", reportPath))
}
