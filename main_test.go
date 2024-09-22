package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
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

var serverOnce sync.Once
var wg sync.WaitGroup
var loremIpsum = "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Donec blandit quam quis odio interdum, ac bibendum elit tincidunt. Lorem ipsum dolor sit amet, consectetur adipiscing elit. Vestibulum nec justo a lacus egestas tincidunt. Curabitur nec nisi laoreet enim tempus vulputate ut et felis. Fusce hendrerit urna lacus, sit amet auctor metus varius id. Curabitur luctus sem vitae ante dapibus ornare. Maecenas dignissim ultrices odio a viverra. Donec fermentum risus ac rutrum fermentum. Pellentesque eu quam iaculis, imperdiet sem ac, posuere dui. Suspendisse consequat dolor nisi, eu semper ligula porttitor ut. Nulla tempus eros erat, ut facilisis enim eleifend non. Praesent accumsan metus est, sed gravida purus placerat in. Curabitur et faucibus arcu. Proin velit urna, vehicula id lacus non, luctus semper diam. Ut porttitor mollis elit, et auctor felis.\nMorbi consequat malesuada mi quis bibendum. Curabitur sed arcu eros. Donec id nunc enim. Sed blandit libero sed sodales viverra. Aenean viverra vitae metus nec finibus. Pellentesque viverra pretium turpis, quis feugiat lacus. Cras aliquet eros augue, at dignissim orci accumsan nec. Pellentesque arcu orci, scelerisque eu congue non, aliquet sit amet elit. Cras pretium metus efficitur velit fringilla, id maximus mi euismod."

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
	http.HandleFunc("/dynamic", func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		response := `<html><body><h1>Time</h1><p>` + loremIpsum + `</p><div>` + time.Now().String() + `</div><p>` + loremIpsum + `</p></body></html>`

		for key := range hiddenParams {
			if queryParams.Get(key) != "" {
				response = `<html><body><h2>Secret</h2></body></html>`
				break
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	})
	http.HandleFunc("/dynamic-reflected", func(w http.ResponseWriter, r *http.Request) {
		queryParams := r.URL.Query()
		response := `<html><body><h1>Time</h1> <p>` + loremIpsum + `</p><div>` + time.Now().String() + `</div><p>` + loremIpsum + `</p><p>` + loremIpsum + `</p></body></html>`

		for key, value := range hiddenParams {
			if queryParams.Get(key) != "" {
				response = `<html><body><h2>Your value</h2>` + value + `<div></div></body></html>`
				break
			}
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(response))
	})
	http.HandleFunc("/unstable", func(w http.ResponseWriter, r *http.Request) {
		rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

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
				randomString(rnd.Intn(100)),
				randomString(rnd.Intn(100)),
				randomString(rnd.Intn(20)),
				randomString(rnd.Intn(50)),
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
	wg.Done()
	log.Fatal(http.ListenAndServe(":8181", nil))
}
func startMockServer() {
	serverOnce.Do(func() {
		wg.Add(1)
		go mockServer()
		wg.Wait()
		time.Sleep(1 * time.Second) // Ensure the server has time to start
	})
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

func TestDiscoverParamsDynamic(t *testing.T) {
	startMockServer()

	params := []string{"page", "param1", "param2", "param3", "param4", "param5", "param6", "query", "session", "user", "token", "mode", "random1", "random2", "random3", "random4", "random5", "player", "team", "score"}

	results := DiscoverParams("http://localhost:8181/dynamic", "GET", "", "", params, 5)

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
	startMockServer()

	params := []string{"param1", "param2", "param3", "param4", "param5", "param6", "page", "query", "session", "user", "token", "mode", "random1", "random2", "random3", "random4", "random5", "player", "team", "score"}

	results := DiscoverParams("http://localhost:8181/dynamic-reflected", "GET", "", "", params, 5)

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
	startMockServer()

	params := []string{"param1", "param2", "param3", "random1", "random2", "user", "token"}

	results := DiscoverParams("http://localhost:8181/unstable", "GET", "", "", params, 5)

	if !results.Aborted {
		t.Errorf("Expected the scan to be aborted due to inconsistent responses, but it was not.")
	}

	if len(results.Params) > 0 || len(results.FormParams) > 0 {
		t.Errorf("Expected no parameters to be reported since the scan was aborted, but found Params: %v, FormParams: %v", results.Params, results.FormParams)
	}
}
