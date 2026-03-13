package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

var hiddenParams = map[string]string{
	"page":    "1",
	"query":   "search",
	"session": "abc123",
	"user":    "admin",
	"token":   "xyz789",
	"mode":    "debug",
}

var loremIpsum = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Donec blandit quam quis odio interdum, ac bibendum elit tincidunt. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vestibulum nec justo a lacus egestas tincidunt. Curabitur nec nisi laoreet enim tempus vulputate ut et felis. Fusce hendrerit urna lacus, sit amet auctor metus varius id. Curabitur luctus sem vitae ante dapibus ornare. Maecenas dignissim ultrices odio a viverra. Donec fermentum risus ac rutrum fermentum. Pellentesque eu quam iaculis, imperdiet sem ac, posuere dui. Suspendisse consequat dolor nisi, eu semper ligula porttitor ut. Nulla tempus eros erat, ut facilisis enim eleifend non. Praesent accumsan metus est, sed gravida purus placerat in. Curabitur et faucibus arcu. Proin velit urna, vehicula id lacus non, luctus semper diam. Ut porttitor mollis elit, et auctor felis.\nMorbi consequat malesuada mi quis bibendum. Curabitur sed arcu eros. Donec id nunc enim. Sed blandit libero sed sodales viverra. Aenean viverra vitae metus nec finibus. Pellentesque viverra pretium turpis, quis feugiat lacus. Cras aliquet eros augue, at dignissim orci accumsan nec. Pellentesque arcu orci, scelerisque eu congue non, aliquet sit amet elit. Cras pretium metus efficitur velit fringilla, id maximus mi euismod."

func createTestMux() *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		response := `<html><body><h1>Normal</h1><form><input name="debug" value="1"><input name="test"/></form></body></html>`

		for key, value := range hiddenParams {
			if queryParams.Get(key) != "" {
				response = `<html><body><h1>Hidden Parameter Detected</h1>` + value + `</body></html>`
				break
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	})

	mux.HandleFunc("/dynamic", func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		rnd := rand.New(rand.NewSource(rand.Int63()))
		noise := randomString(10 + rnd.Intn(20))
		response := `<html><body><h1>Time</h1><p>` + loremIpsum + `</p><div>` + noise + `</div><p>` + loremIpsum + `</p></body></html>`

		for key := range hiddenParams {
			if queryParams.Get(key) != "" {
				response = `<html><body><h2>Secret</h2></body></html>`
				break
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	})

	mux.HandleFunc("/dynamic-reflected", func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		rnd := rand.New(rand.NewSource(rand.Int63()))
		noise := randomString(10 + rnd.Intn(20))
		response := `<html><body><h1>Time</h1> <p>` + loremIpsum + `</p><div>` + noise + `</div><p>` + loremIpsum + `</p><p>` + loremIpsum + `</p></body></html>`

		for key, value := range hiddenParams {
			if queryParams.Get(key) != "" {
				response = `<html><body><h2>Your value</h2>` + value + `<div></div></body></html>`
				break
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	})

	mux.HandleFunc("/unstable", func(w http.ResponseWriter, r *http.Request) {
		rnd := rand.New(rand.NewSource(rand.Int63()))

		numElements := rnd.Intn(5) + 1
		response := "<html><body>"

		for i := 0; i < numElements; i++ {
			tags := []string{"h1", "h2", "p", "div", "span", "ul", "ol", "li", "a", "strong"}
			tag := tags[rnd.Intn(len(tags))]

			contentOptions := []string{
				"Random Content " + strconv.Itoa(rnd.Int()),
				"Another Random String " + strconv.Itoa(rnd.Int()),
				"More Random Text " + strconv.Itoa(rnd.Int()),
				loremIpsum[:rnd.Intn(len(loremIpsum))],
				randomString(rnd.Intn(100) + 1),
				randomString(rnd.Intn(100) + 1),
				randomString(rnd.Intn(20) + 1),
				randomString(rnd.Intn(50) + 1),
				loremIpsum[:rnd.Intn(len(loremIpsum))],
			}
			content := contentOptions[rnd.Intn(len(contentOptions))]

			element := fmt.Sprintf("<%s>%s</%s>", tag, content, tag)
			response += element
		}

		response += "</body></html>"
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	})

	mux.HandleFunc("/with-auth", func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized"))
			return
		}

		response := `<html><body><h1>Authenticated</h1></body></html>`
		for key := range hiddenParams {
			if queryParams.Get(key) != "" {
				response = `<html><body><h1>Auth + Hidden</h1></body></html>`
				break
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	})

	return mux
}

func newTestConfig() *Config {
	return &Config{
		Timeout:             10,
		Concurrency:         10,
		SimilarityThreshold: 0.9,
		httpClient:          createHTTPClient(10, false),
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func TestDiscoverParams(t *testing.T) {
	server := httptest.NewServer(createTestMux())
	defer server.Close()

	cfg := newTestConfig()
	params := []string{"param1", "param2", "param3", "param4", "param5", "param6", "page", "query", "session", "user", "token", "mode", "random1", "random2", "random3", "random4", "random5", "player", "team", "score"}

	request := Request{
		URL:    server.URL,
		Method: "GET",
	}

	results := DiscoverParams(cfg, request, params, 5)

	expectedParams := []string{"page", "query", "session", "user", "token", "mode"}

	for _, param := range expectedParams {
		if !contains(results.Params, param) {
			t.Errorf("Expected parameter %s not found. Detected: %s", param, results.Params)
		}
	}

	for _, param := range []string{"debug", "test"} {
		if !contains(results.FormParams, param) {
			t.Errorf("Expected form parameter %s not found. Detected: %s", param, results.FormParams)
		}
	}

	if contains(results.Params, "test") {
		t.Errorf("Reported form parameter 'test' which does not modify the content as valid")
	}
}

func TestDiscoverParamsDynamic(t *testing.T) {
	server := httptest.NewServer(createTestMux())
	defer server.Close()

	cfg := newTestConfig()
	params := []string{"page", "param1", "param2", "param3", "param4", "param5", "param6", "query", "session", "user", "token", "mode", "random1", "random2", "random3", "random4", "random5", "player", "team", "score"}

	request := Request{
		URL:    server.URL + "/dynamic",
		Method: "GET",
	}

	results := DiscoverParams(cfg, request, params, 5)

	expectedParams := []string{"page", "query", "session", "user", "token", "mode"}

	for _, param := range expectedParams {
		if !contains(results.Params, param) {
			t.Errorf("Expected parameter %s not found. Detected: %s", param, results.Params)
		}
	}

	if contains(results.Params, "param1") {
		t.Errorf("Reported form parameter 'param1' which does not modify the content as valid")
	}
	if contains(results.Params, "random1") {
		t.Errorf("Reported form parameter 'random1' which does not modify the content as valid")
	}
	if contains(results.Params, "team") {
		t.Errorf("Reported form parameter 'team' which does not modify the content as valid")
	}
}

func TestDiscoverParamsDynamicReflected(t *testing.T) {
	server := httptest.NewServer(createTestMux())
	defer server.Close()

	cfg := newTestConfig()
	params := []string{"param1", "param2", "param3", "param4", "param5", "param6", "page", "query", "session", "user", "token", "mode", "random1", "random2", "random3", "random4", "random5", "player", "team", "score"}

	request := Request{
		URL:    server.URL + "/dynamic-reflected",
		Method: "GET",
	}

	results := DiscoverParams(cfg, request, params, 5)

	expectedParams := []string{"page", "query", "session", "user", "token", "mode"}
	for _, param := range expectedParams {
		if !contains(results.Params, param) {
			t.Errorf("Expected parameter %s not found. Detected: %s", param, results.Params)
		}
	}

	if contains(results.Params, "random1") {
		t.Errorf("Reported form parameter 'random1' which does not modify the content as valid")
	}
	if contains(results.Params, "param6") {
		t.Errorf("Reported form parameter 'param6' which does not modify the content as valid")
	}
	if contains(results.Params, "team") {
		t.Errorf("Reported form parameter 'team' which does not modify the content as valid")
	}
}

func TestDiscoverParamsDynamicUnstable(t *testing.T) {
	server := httptest.NewServer(createTestMux())
	defer server.Close()

	cfg := newTestConfig()
	params := []string{"param1", "param2", "param3", "random1", "random2", "user", "token"}

	request := Request{
		URL:    server.URL + "/unstable",
		Method: "GET",
	}

	results := DiscoverParams(cfg, request, params, 5)

	if !results.Aborted {
		t.Errorf("Expected the scan to be aborted due to inconsistent responses, but it was not.")
	}

	if len(results.Params) > 0 || len(results.FormParams) > 0 {
		t.Errorf("Expected no parameters to be reported since the scan was aborted, but found Params: %v, FormParams: %v", results.Params, results.FormParams)
	}
}

func TestCustomHeaders(t *testing.T) {
	server := httptest.NewServer(createTestMux())
	defer server.Close()

	cfg := newTestConfig()
	cfg.Headers = []string{"Authorization: Bearer test-token"}

	params := []string{"param1", "page", "query"}
	request := Request{
		URL:    server.URL + "/with-auth",
		Method: "GET",
	}

	results := DiscoverParams(cfg, request, params, 5)

	if results.Aborted {
		t.Error("Expected scan to succeed with auth header, but it was aborted")
	}

	for _, expected := range []string{"page", "query"} {
		if !contains(results.Params, expected) {
			t.Errorf("Expected parameter %s not found with custom headers. Detected: %v", expected, results.Params)
		}
	}

	if contains(results.Params, "param1") {
		t.Errorf("False positive: param1 should not be in valid params")
	}
}

func TestCustomHeadersWithoutAuth(t *testing.T) {
	server := httptest.NewServer(createTestMux())
	defer server.Close()

	cfg := newTestConfig()
	// No auth header set

	params := []string{"param1", "page", "query"}
	request := Request{
		URL:    server.URL + "/with-auth",
		Method: "GET",
	}

	results := DiscoverParams(cfg, request, params, 5)

	// Without auth, baseline is 401 "Unauthorized"
	// Sending hidden params still returns 401, so no params should be detected
	// (the server checks auth before checking params)
	if len(results.Params) > 0 {
		t.Errorf("Expected no valid params without auth header, got: %v", results.Params)
	}
}

func TestWordlistDedup(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "wordlist-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	content := "param1\nparam2\nparam1\nparam3\nparam2\nparam1\n"
	tmpFile.WriteString(content)
	tmpFile.Close()

	params := loadWordlist(tmpFile.Name())
	if len(params) != 3 {
		t.Errorf("Expected 3 unique params after dedup, got %d: %v", len(params), params)
	}

	expected := []string{"param1", "param2", "param3"}
	for i, p := range expected {
		if params[i] != p {
			t.Errorf("Expected params[%d] = %s, got %s", i, p, params[i])
		}
	}
}

func TestFormParamFalsePositives(t *testing.T) {
	server := httptest.NewServer(createTestMux())
	defer server.Close()

	cfg := newTestConfig()

	// Only non-hidden params in wordlist
	params := []string{"param1", "param2", "param3"}
	request := Request{
		URL:    server.URL,
		Method: "GET",
	}

	results := DiscoverParams(cfg, request, params, 5)

	// Form params should be extracted
	for _, fp := range []string{"debug", "test"} {
		if !contains(results.FormParams, fp) {
			t.Errorf("Expected form param '%s' to be extracted", fp)
		}
	}

	// Neither wordlist params nor form-only params should be valid
	// (none of them trigger hidden behavior)
	for _, param := range []string{"param1", "param2", "param3", "test", "debug"} {
		if contains(results.Params, param) {
			t.Errorf("False positive: '%s' should not be in valid params", param)
		}
	}
}

func TestTotalRequestsAtomic(t *testing.T) {
	server := httptest.NewServer(createTestMux())
	defer server.Close()

	cfg := newTestConfig()
	params := []string{"param1", "page"}
	request := Request{
		URL:    server.URL,
		Method: "GET",
	}

	// Reset counter
	cfg.totalRequests.Store(0)
	DiscoverParams(cfg, request, params, 5)

	count := cfg.totalRequests.Load()
	if count == 0 {
		t.Error("Expected totalRequests to be incremented")
	}

	// Run again with a fresh config to verify isolation
	cfg2 := newTestConfig()
	cfg2.totalRequests.Store(0)
	DiscoverParams(cfg2, request, params, 5)

	count2 := cfg2.totalRequests.Load()
	if count2 == 0 {
		t.Error("Expected totalRequests to be incremented on second config")
	}

	// Verify configs are independent
	if cfg.totalRequests.Load() != count {
		t.Error("First config's totalRequests was modified by second run")
	}
}
