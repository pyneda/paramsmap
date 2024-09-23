package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
)

var totalRequests int
var logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
var ignoreCertErrors bool
var numBaselines = 3
var reportPath string

func main() {
	var requestURL, method, postData, contentType, wordlist string
	var chunkSize int
	flag.StringVar(&requestURL, "url", "", "The URL to make the request to")
	flag.StringVar(&method, "method", "GET", "HTTP method to use")
	flag.StringVar(&postData, "data", "", "Optional POST data")
	flag.StringVar(&contentType, "type", "form", "Content type: form, json, xml")
	flag.StringVar(&wordlist, "wordlist", "wordlist.txt", "Path to the wordlist file")
	flag.StringVar(&reportPath, "report", "report.json", "Path to the output report file")
	flag.IntVar(&chunkSize, "chunk-size", 1000, "Number of parameters to send in each request")
	flag.BoolVar(&ignoreCertErrors, "ignore-cert", false, "Ignore SSL certificate errors")

	flag.Parse()

	if requestURL == "" {
		logger.Error("URL is required")
		return
	}

	params := loadWordlist(wordlist)
	logger.Info("Loaded parameters from wordlist", "count", len(params))
	request := Request{
		URL:         requestURL,
		Method:      method,
		Data:        postData,
		ContentType: contentType,
	}
	results := DiscoverParams(request, params, chunkSize)
	logger.Info("Total requests made", "count", totalRequests)
	logger.Info("Valid parameters found", "count", len(results.Params), "valid", results.Params)
	logger.Info("Form parameters found", "count", len(results.FormParams), "parameters", results.FormParams)
	if reportPath != "" {
		saveReport(reportPath, results)
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
	StatusCode  int
	Reflections int
}

type InitialResponses struct {
	Responses     []ResponseData
	SameBody      bool
	AreConsistent bool
}

func DiscoverParams(request Request, params []string, chunkSize int) Results {
	initialResponses := makeInitialRequests(request)

	// Check if baseline responses are consistent
	if !initialResponses.AreConsistent {
		logger.Warn("Baseline responses differ significantly. The page appears to be too dynamic. Scanning will be skipped.")
		return Results{
			Params:        []string{},
			FormParams:    []string{},
			Aborted:       true,
			AbortReason:   "Baseline responses differ significantly",
			TotalRequests: totalRequests,
			Request:       request,
		}
	}

	formsParams := extractFormParams(initialResponses.Responses[0].Body)
	logger.Info("Extracted form parameters", "count", len(formsParams), "parameters", formsParams)

	params = append(params, formsParams...)
	validParams := discoverValidParams(request, params, initialResponses, chunkSize)
	return Results{
		Params:        validParams,
		FormParams:    formsParams,
		TotalRequests: totalRequests,
		Request:       request,
	}
}

func discoverValidParams(request Request, params []string, initialResponses InitialResponses, chunkSize int) []string {
	parts := chunkParams(params, chunkSize)
	validParts := filterParts(request, parts, initialResponses)

	paramSet := make(map[string]bool)
	var validParams []string
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, part := range validParts {
		wg.Add(1)
		go func(part []string) {
			defer wg.Done()
			for _, param := range recursiveFilter(request, part, initialResponses) {
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

func filterParts(request Request, parts [][]string, initialResponses InitialResponses) [][]string {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var validParts [][]string

	for _, part := range parts {
		wg.Add(1)
		go func(part []string) {
			defer wg.Done()
			params := generateParams(part)
			response := makeRequest(request, params)

			if responseChanged(initialResponses.Responses, response, initialResponses.SameBody) {
				mu.Lock()
				validParts = append(validParts, part)
				mu.Unlock()
			}
		}(part)
	}
	wg.Wait()
	return validParts
}

func recursiveFilter(request Request, params []string, initialResponses InitialResponses) []string {
	if len(params) == 1 {
		return params
	}
	mid := len(params) / 2
	left := params[:mid]
	right := params[mid:]

	leftParams := generateParams(left)
	rightParams := generateParams(right)

	leftResponse := makeRequest(request, leftParams)
	rightResponse := makeRequest(request, rightParams)

	var validParams []string
	if responseChanged(initialResponses.Responses, leftResponse, initialResponses.SameBody) {
		validParams = append(validParams, recursiveFilter(request, left, initialResponses)...)
	}
	if responseChanged(initialResponses.Responses, rightResponse, initialResponses.SameBody) {
		validParams = append(validParams, recursiveFilter(request, right, initialResponses)...)
	}
	return validParams
}

func makeInitialRequests(request Request) InitialResponses {
	var baselineResponses []ResponseData
	for i := 0; i < numBaselines; i++ {
		resp := makeRequest(request, url.Values{})
		baselineResponses = append(baselineResponses, resp)
	}

	return InitialResponses{
		Responses:     baselineResponses,
		SameBody:      baselineResponsesAreConsistent(baselineResponses, responsesAreEqual),
		AreConsistent: baselineResponsesAreConsistent(baselineResponses, responsesAreSimilar),
	}
}

func makeRequest(request Request, params url.Values) ResponseData {
	var req *http.Request
	var err error
	totalRequests++
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
	if request.Method == "GET" {
		req, err = http.NewRequest(request.Method, requestURL, nil)
	} else {
		var body []byte
		if request.ContentType == "json" {
			body = []byte(request.Data)
			req, err = http.NewRequest(request.Method, requestURL, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
		} else if request.ContentType == "xml" {
			body = []byte(request.Data)
			req, err = http.NewRequest(request.Method, requestURL, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/xml")
		} else {
			req, err = http.NewRequest(request.Method, requestURL, strings.NewReader(params.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
	}

	req.Header.Set("User-Agent", randomUserAgent())
	if err != nil {
		logger.Error("Failed to create request", "error", err)
	}

	client := createHTTPClient()
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to make request", "error", err)
		return ResponseData{}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response body", "error", err)
	}

	reflections := countReflections(params, body)
	return ResponseData{Body: body, StatusCode: resp.StatusCode, Reflections: reflections}
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
