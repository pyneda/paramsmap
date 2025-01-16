package main

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/PuerkitoBio/goquery"
)

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

func createHTTPClient(timeout int) *http.Client {
	if ignoreCertErrors {
		tr := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		return &http.Client{Transport: tr, Timeout: time.Duration(timeout) * time.Second}
	}
	return &http.Client{}
}

func generateParams(params []string) url.Values {
	values := url.Values{}
	for _, param := range params {
		values.Set(param, randomString(8))
	}
	return values
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

func randomString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
