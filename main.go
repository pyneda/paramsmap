package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"flag"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
)

var totalRequests int
var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
var ignoreCertErrors bool

func main() {
	var requestURL, method, postData, contentType, wordlist string
	var chunkSize int
	flag.StringVar(&requestURL, "url", "", "The URL to make the request to")
	flag.StringVar(&method, "method", "GET", "HTTP method to use")
	flag.StringVar(&postData, "data", "", "Optional POST data")
	flag.StringVar(&contentType, "type", "form", "Content type: form, json, xml")
	flag.StringVar(&wordlist, "wordlist", "wordlist.txt", "Path to the wordlist file")
	flag.IntVar(&chunkSize, "chunk-size", 1000, "Number of parameters to send in each request")
	flag.BoolVar(&ignoreCertErrors, "ignore-cert", false, "Ignore SSL certificate errors")

	flag.Parse()

	if requestURL == "" {
		logger.Error("URL is required")
		return
	}

	params := loadWordlist(wordlist)
	logger.Info("Loaded parameters from wordlist", "count", len(params))
	results := DiscoverParams(requestURL, method, postData, contentType, params, chunkSize)
	logger.Info("Total requests made", "count", totalRequests)
	logger.Info("Valid parameters found", "count", len(results.Params), "valid", results.Params)

}

type Results struct {
	Params     []string `json:"params"`
	FormParams []string `json:"form_params"`
}

func DiscoverParams(requestURL, method, postData, contentType string, params []string, chunkSize int) Results {
	initialResponses := makeInitialRequests(requestURL, method, postData, contentType)
	logger.Info("Initial responses status codes", "first", initialResponses[0].StatusCode, "second", initialResponses[1].StatusCode)
	formsParams := extractFormParams(initialResponses[0].Body)
	logger.Info("Extracted form parameters", "count", len(formsParams), "parameters", formsParams)

	params = append(params, formsParams...)
	validParams := discoverValidParams(requestURL, method, postData, contentType, params, initialResponses, chunkSize)
	return Results{
		Params:     validParams,
		FormParams: formsParams,
	}
}

type ResponseData struct {
	Body        []byte
	StatusCode  int
	Reflections int
}

func randomUserAgent() string {
	userAgents := []string{
		"Mozilla/5.0 (Windows NT 6.1; WOW64; rv:40.0) Gecko/20100101 Firefox/40.1",
		"Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/44.0.2403.157 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0",
	}
	return userAgents[rand.Intn(len(userAgents))]
}

func loadWordlist(wordlist string) []string {
	file, err := os.Open(wordlist)
	if err != nil {
		logger.Error("Failed to open wordlist", "error", err)
		os.Exit(1)
	}
	defer file.Close()

	var params []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		params = append(params, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		logger.Error("Failed to read wordlist", "error", err)
		os.Exit(1)
	}

	return params
}

func makeInitialRequests(requestURL, method, postData, contentType string) []ResponseData {
	paramSet1 := url.Values{"param1": {randomString(5)}, "param2": {randomString(5)}}
	paramSet2 := url.Values{"param3": {randomString(5)}, "param4": {randomString(5)}}

	resp1 := makeRequest(requestURL, method, postData, contentType, paramSet1)
	resp2 := makeRequest(requestURL, method, postData, contentType, paramSet2)

	return []ResponseData{resp1, resp2}
}

func makeRequest(requestURL, method, postData, contentType string, params url.Values) ResponseData {
	var req *http.Request
	var err error
	totalRequests++
	parsedURL, err := url.Parse(requestURL)
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
	requestURL = parsedURL.String()
	if method == "GET" {
		req, err = http.NewRequest(method, requestURL, nil)
	} else {
		var body []byte
		if contentType == "json" {
			body = []byte(postData)
			req, err = http.NewRequest(method, requestURL, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
		} else if contentType == "xml" {
			body = []byte(postData)
			req, err = http.NewRequest(method, requestURL, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/xml")
		} else {
			req, err = http.NewRequest(method, requestURL, strings.NewReader(params.Encode()))
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

func countReflections(params url.Values, body []byte) int {
	count := 0
	for _, values := range params {
		for _, value := range values {
			if bytes.Contains(body, []byte(value)) {
				count++
			}
		}
	}
	return count
}

func createHTTPClient() *http.Client {
	if ignoreCertErrors {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		return &http.Client{Transport: tr}
	}
	return &http.Client{}
}

func extractFormParams(body []byte) []string {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		logger.Error("Failed to parse HTML", "error", err)
	}

	var formParams []string
	doc.Find("input, select, textarea").Each(func(i int, s *goquery.Selection) {
		name, exists := s.Attr("name")
		if exists {
			formParams = append(formParams, name)
		}
	})
	return formParams
}

func discoverValidParams(requestURL, method, postData, contentType string, params []string, initialResponses []ResponseData, chunkSize int) []string {
	parts := chunkParams(params, chunkSize)
	validParts := filterParts(requestURL, method, postData, contentType, parts, initialResponses[0])

	paramSet := make(map[string]bool)
	var validParams []string
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, part := range validParts {
		wg.Add(1)
		go func(part []string) {
			defer wg.Done()
			for _, param := range recursiveFilter(requestURL, method, postData, contentType, part, initialResponses[0]) {
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

func chunkParams(params []string, chunkSize int) [][]string {
	var chunks [][]string
	for i := 0; i < len(params); i += chunkSize {
		end := i + chunkSize
		if end > len(params) {
			end = len(params)
		}
		chunks = append(chunks, params[i:end])
	}
	return chunks
}

func filterParts(requestURL, method, postData, contentType string, parts [][]string, initialResponse ResponseData) [][]string {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var validParts [][]string

	for _, part := range parts {
		wg.Add(1)
		go func(part []string) {
			defer wg.Done()
			params := generateParams(part)
			response := makeRequest(requestURL, method, postData, contentType, params)

			if responseChanged(initialResponse, response) {
				mu.Lock()
				validParts = append(validParts, part)
				mu.Unlock()
			}
		}(part)
	}
	wg.Wait()
	return validParts
}

func generateParams(params []string) url.Values {
	values := url.Values{}
	for _, param := range params {
		values.Set(param, randomString(8))
	}
	return values
}

func randomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func responseChanged(initial, new ResponseData) bool {
	return initial.StatusCode != new.StatusCode || len(initial.Body) != len(new.Body) || initial.Reflections != new.Reflections
}

func recursiveFilter(requestURL, method, postData, contentType string, params []string, initialResponse ResponseData) []string {
	if len(params) == 1 {
		return params
	}
	mid := len(params) / 2
	left := params[:mid]
	right := params[mid:]

	leftParams := generateParams(left)
	rightParams := generateParams(right)

	leftResponse := makeRequest(requestURL, method, postData, contentType, leftParams)
	rightResponse := makeRequest(requestURL, method, postData, contentType, rightParams)

	var validParams []string
	if responseChanged(initialResponse, leftResponse) {
		validParams = append(validParams, recursiveFilter(requestURL, method, postData, contentType, left, initialResponse)...)
	}
	if responseChanged(initialResponse, rightResponse) {
		validParams = append(validParams, recursiveFilter(requestURL, method, postData, contentType, right, initialResponse)...)
	}
	return validParams
}
