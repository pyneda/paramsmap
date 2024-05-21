package main

import (
	"log"
	"net/http"
	"testing"
	"time"
)

var hiddenParams = map[string]string{
	"page":    "1",
	"query":   "search",
	"session": "abc123",
	"user":    "admin",
	"token":   "xyz789",
	"mode":    "debug",
}

func mockServer() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		response := `<html><body><h1>Normal</h1><form><input name="debug" value="1"><input name="test"/></body></html>`

		for key, value := range hiddenParams {
			if queryParams.Get(key) != "" {
				response = `<html><body><h1>Hidden Parameter Detected</h1>` + value + `</body></html>`
				break
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	})

	log.Fatal(http.ListenAndServe(":8181", nil))
}

func startMockServer() {
	go mockServer()
	time.Sleep(1 * time.Second)
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
	startMockServer()

	params := []string{"param1", "param2", "param3", "param4", "param5", "param6", "page", "query", "session", "user", "token", "mode", "random1", "random2", "random3", "random4", "random5", "player", "team", "score"}

	results := DiscoverParams("http://localhost:8181", "GET", "", "", params, 5)

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
