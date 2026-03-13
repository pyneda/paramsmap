package main

import (
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestChunkParams(t *testing.T) {
	tests := []struct {
		name      string
		params    []string
		chunkSize int
		wantLen   int
		wantLast  int // expected length of last chunk
	}{
		{"even split", []string{"a", "b", "c", "d"}, 2, 2, 2},
		{"uneven split", []string{"a", "b", "c", "d", "e"}, 2, 3, 1},
		{"chunk larger than params", []string{"a", "b"}, 10, 1, 2},
		{"chunk size 1", []string{"a", "b", "c"}, 1, 3, 1},
		{"empty params", []string{}, 5, 0, 0},
		{"single param", []string{"a"}, 1, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := chunkParams(tt.params, tt.chunkSize)
			if len(chunks) != tt.wantLen {
				t.Errorf("got %d chunks, want %d", len(chunks), tt.wantLen)
			}
			if tt.wantLen > 0 && len(chunks[len(chunks)-1]) != tt.wantLast {
				t.Errorf("last chunk has %d items, want %d", len(chunks[len(chunks)-1]), tt.wantLast)
			}
			// Verify all params are present
			var all []string
			for _, chunk := range chunks {
				all = append(all, chunk...)
			}
			if len(all) != len(tt.params) {
				t.Errorf("chunks contain %d total params, want %d", len(all), len(tt.params))
			}
		})
	}
}

func TestLoadWordlist(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"basic", "param1\nparam2\nparam3\n", []string{"param1", "param2", "param3"}},
		{"with duplicates", "a\nb\na\nc\nb\n", []string{"a", "b", "c"}},
		{"empty lines", "a\n\nb\n\n\nc\n", []string{"a", "b", "c"}},
		{"whitespace trimming", "  a  \n\tb\t\n c \n", []string{"a", "b", "c"}},
		{"single param", "only\n", []string{"only"}},
		{"no trailing newline", "a\nb", []string{"a", "b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "wordlist-*.txt")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpFile.Name())
			tmpFile.WriteString(tt.content)
			tmpFile.Close()

			got := loadWordlist(tmpFile.Name())
			if len(got) != len(tt.want) {
				t.Errorf("got %d params %v, want %d %v", len(got), got, len(tt.want), tt.want)
				return
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("param[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestRandomString(t *testing.T) {
	// Length
	for _, n := range []int{0, 1, 8, 100} {
		s := randomString(n)
		if len(s) != n {
			t.Errorf("randomString(%d) produced string of length %d", n, len(s))
		}
	}

	// Only alpha chars
	s := randomString(1000)
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			t.Errorf("randomString produced non-alpha character: %c", c)
		}
	}

	// Not always the same (probabilistic but effectively guaranteed)
	a := randomString(32)
	b := randomString(32)
	if a == b {
		t.Error("two random strings of length 32 should differ")
	}
}

func TestCountReflections(t *testing.T) {
	tests := []struct {
		name   string
		params url.Values
		body   string
		want   int
	}{
		{
			"no reflections",
			url.Values{"key": {"uniqueVal123"}},
			"<html><body>no match here</body></html>",
			0,
		},
		{
			"one reflection",
			url.Values{"key": {"reflected"}},
			"<html><body>your input: reflected</body></html>",
			1,
		},
		{
			"multiple params reflected",
			url.Values{"a": {"alpha"}, "b": {"beta"}},
			"<html>alpha and beta found</html>",
			2,
		},
		{
			"same param multiple values",
			url.Values{"key": {"val1", "val2"}},
			"<html>val1 and val2</html>",
			2,
		},
		{
			"partial match still counts",
			url.Values{"key": {"test"}},
			"<html>testing this</html>",
			1, // "test" is contained in "testing"
		},
		{
			"empty body",
			url.Values{"key": {"val"}},
			"",
			0,
		},
		{
			"empty params",
			url.Values{},
			"<html>anything</html>",
			0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := countReflections(tt.params, []byte(tt.body))
			if got != tt.want {
				t.Errorf("countReflections() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCreateHTTPClient(t *testing.T) {
	// Normal client has timeout
	client := createHTTPClient(5, false)
	if client.Timeout.Seconds() != 5 {
		t.Errorf("expected 5s timeout, got %v", client.Timeout)
	}
	if client.Transport != nil {
		t.Error("normal client should not have custom transport")
	}

	// Insecure client has timeout and custom transport
	insecure := createHTTPClient(10, true)
	if insecure.Timeout.Seconds() != 10 {
		t.Errorf("expected 10s timeout, got %v", insecure.Timeout)
	}
	if insecure.Transport == nil {
		t.Error("insecure client should have custom transport")
	}

	// Zero timeout
	zero := createHTTPClient(0, false)
	if zero.Timeout.Seconds() != 0 {
		t.Errorf("expected 0s timeout, got %v", zero.Timeout)
	}
}

func TestGenerateParams(t *testing.T) {
	params := []string{"key1", "key2", "key3"}
	values := generateParams(params)

	// All keys present
	for _, key := range params {
		val := values.Get(key)
		if val == "" {
			t.Errorf("expected key %q to have a value", key)
		}
		if len(val) != 8 {
			t.Errorf("expected value length 8, got %d for key %q", len(val), key)
		}
	}

	// No extra keys
	if len(values) != len(params) {
		t.Errorf("expected %d keys, got %d", len(params), len(values))
	}

	// Empty input
	empty := generateParams([]string{})
	if len(empty) != 0 {
		t.Errorf("expected empty values for empty input, got %d", len(empty))
	}
}

func TestExtractFormParams(t *testing.T) {
	tests := []struct {
		name string
		html string
		want []string
	}{
		{
			"input fields",
			`<html><form><input name="user"><input name="pass"></form></html>`,
			[]string{"user", "pass"},
		},
		{
			"select and textarea",
			`<html><form><select name="country"></select><textarea name="bio"></textarea></form></html>`,
			[]string{"country", "bio"},
		},
		{
			"input without name",
			`<html><form><input type="submit"><input name="real"></form></html>`,
			[]string{"real"},
		},
		{
			"no form elements",
			`<html><body><p>no forms here</p></body></html>`,
			nil,
		},
		{
			"nested forms",
			`<html><form><div><input name="nested"></div></form></html>`,
			[]string{"nested"},
		},
		{
			"mixed elements",
			`<html><form><input name="a" value="1"><input name="b"/><select name="c"><option>1</option></select><textarea name="d"></textarea></form></html>`,
			[]string{"a", "b", "c", "d"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFormParams([]byte(tt.html))
			if len(got) != len(tt.want) {
				t.Errorf("got %d params %v, want %d %v", len(got), got, len(tt.want), tt.want)
				return
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("param[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestRandomUserAgent(t *testing.T) {
	ua := randomUserAgent()
	if ua == "" {
		t.Error("randomUserAgent should not return empty string")
	}
	if !strings.HasPrefix(ua, "Mozilla/5.0") {
		t.Errorf("unexpected user agent format: %s", ua)
	}
}
