package main

import (
	"crypto/sha256"
	"testing"
)

func makeResponseData(body string, statusCode int, reflections int) ResponseData {
	b := []byte(body)
	return ResponseData{
		Body:        b,
		BodyHash:    sha256.Sum256(b),
		StatusCode:  statusCode,
		Reflections: reflections,
	}
}

func TestComputeSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		minSim   float64
		maxSim   float64
	}{
		{"identical", "hello world", "hello world", 1.0, 1.0},
		{"both empty", "", "", 1.0, 1.0},
		{"completely different", "aaaa", "zzzz", 0.0, 0.01},
		{"one empty", "hello", "", 0.0, 0.01},
		{"minor change", "hello world foo bar", "hello world foo baz", 0.85, 1.0},
		{"major change", "abcdef", "xyz", 0.0, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sim := computeSimilarity([]byte(tt.a), []byte(tt.b))
			if sim < tt.minSim || sim > tt.maxSim {
				t.Errorf("computeSimilarity(%q, %q) = %f, want [%f, %f]", tt.a, tt.b, sim, tt.minSim, tt.maxSim)
			}
		})
	}
}

func TestResponsesAreEqual(t *testing.T) {
	a := makeResponseData("hello", 200, 0)
	b := makeResponseData("hello", 200, 0)
	if !responsesAreEqual(a, b) {
		t.Error("identical responses should be equal")
	}

	// Different body, same length
	c := makeResponseData("hellx", 200, 0)
	if responsesAreEqual(a, c) {
		t.Error("same-length different-content responses should NOT be equal (hash differs)")
	}

	// Different status code
	d := makeResponseData("hello", 404, 0)
	if responsesAreEqual(a, d) {
		t.Error("different status codes should not be equal")
	}

	// Different reflections
	e := makeResponseData("hello", 200, 3)
	if responsesAreEqual(a, e) {
		t.Error("different reflection counts should not be equal")
	}
}

func TestResponsesAreSimilar(t *testing.T) {
	base := makeResponseData("hello world this is a test page with content", 200, 0)

	// Identical
	same := makeResponseData("hello world this is a test page with content", 200, 0)
	if !responsesAreSimilar(base, same, 0.9) {
		t.Error("identical responses should be similar")
	}

	// Minor body change (high similarity) - use long strings so a small diff stays above 0.9
	minor := makeResponseData("hello world this is a test page with contenx", 200, 0)
	if !responsesAreSimilar(base, minor, 0.9) {
		sim := computeSimilarity(base.Body, minor.Body)
		t.Errorf("minor body change should still be similar at 0.9 threshold, got similarity=%f", sim)
	}

	// Completely different body
	different := makeResponseData("xyz", 200, 0)
	if responsesAreSimilar(base, different, 0.9) {
		t.Error("completely different body should not be similar")
	}

	// Different status code
	diffStatus := makeResponseData("hello world this is a test page with content", 500, 0)
	if responsesAreSimilar(base, diffStatus, 0.9) {
		t.Error("different status code should not be similar")
	}

	// Different reflections
	diffRef := makeResponseData("hello world this is a test page with content", 200, 5)
	if responsesAreSimilar(base, diffRef, 0.9) {
		t.Error("different reflections should not be similar")
	}

	// Low threshold accepts more difference
	looseDiff := makeResponseData("hello world completely rewritten page", 200, 0)
	if responsesAreSimilar(base, looseDiff, 0.9) {
		t.Error("should not be similar at 0.9")
	}
	if !responsesAreSimilar(base, looseDiff, 0.1) {
		t.Error("should be similar at very low 0.1 threshold")
	}
}

func TestResponseChanged(t *testing.T) {
	baselines := []ResponseData{
		makeResponseData("page content v1", 200, 0),
		makeResponseData("page content v1", 200, 0),
	}

	// Same response - not changed
	same := makeResponseData("page content v1", 200, 0)
	if responseChanged(baselines, same, true, 0.9) {
		t.Error("identical response should not be marked as changed (equalCheck=true)")
	}
	if responseChanged(baselines, same, false, 0.9) {
		t.Error("identical response should not be marked as changed (equalCheck=false)")
	}

	// Different response
	diff := makeResponseData("totally new content", 200, 0)
	if !responseChanged(baselines, diff, true, 0.9) {
		t.Error("different response should be marked as changed (equalCheck=true)")
	}
	if !responseChanged(baselines, diff, false, 0.9) {
		t.Error("different response should be marked as changed (equalCheck=false)")
	}

	// Status code change
	statusDiff := makeResponseData("page content v1", 302, 0)
	if !responseChanged(baselines, statusDiff, true, 0.9) {
		t.Error("status code change should be detected")
	}
}

func TestResponseChangedWithDynamicBaselines(t *testing.T) {
	// Baselines that differ slightly (dynamic page) - long enough that timestamp diff is < 10%
	baselines := []ResponseData{
		makeResponseData("page content with lots of text here for similarity calculation timestamp=111", 200, 0),
		makeResponseData("page content with lots of text here for similarity calculation timestamp=222", 200, 0),
	}

	// Similar to one baseline - not changed (similarity mode)
	similar := makeResponseData("page content with lots of text here for similarity calculation timestamp=333", 200, 0)
	if responseChanged(baselines, similar, false, 0.9) {
		t.Error("similar response should not be changed in similarity mode")
	}

	// Completely different - changed
	diff := makeResponseData("<html><body>Hidden Param Detected</body></html>", 200, 0)
	if !responseChanged(baselines, diff, false, 0.9) {
		t.Error("completely different response should be changed")
	}
}

func TestResponseChangedIgnoringReflections(t *testing.T) {
	baselines := []ResponseData{
		makeResponseData("page with form inputs", 200, 2),
	}

	// Same body but different reflections - should NOT be marked as changed
	sameBodyDiffRef := makeResponseData("page with form inputs", 200, 5)
	if responseChangedIgnoringReflections(baselines, sameBodyDiffRef, true, 0.9) {
		t.Error("same body with different reflections should not be changed when ignoring reflections")
	}

	// Different body - should be changed
	diffBody := makeResponseData("completely different page", 200, 2)
	if !responseChangedIgnoringReflections(baselines, diffBody, true, 0.9) {
		t.Error("different body should be changed even when ignoring reflections")
	}

	// Different status code - should be changed
	diffStatus := makeResponseData("page with form inputs", 404, 2)
	if !responseChangedIgnoringReflections(baselines, diffStatus, true, 0.9) {
		t.Error("different status code should be changed")
	}
}

func TestResponseChangedIgnoringReflectionsDynamic(t *testing.T) {
	baselines := []ResponseData{
		makeResponseData("dynamic page content ts=111 "+loremIpsum, 200, 0),
		makeResponseData("dynamic page content ts=222 "+loremIpsum, 200, 0),
	}

	// Similar body, different reflections - not changed
	similar := makeResponseData("dynamic page content ts=333 "+loremIpsum, 200, 3)
	if responseChangedIgnoringReflections(baselines, similar, false, 0.9) {
		t.Error("similar body should not be changed on dynamic page when ignoring reflections")
	}

	// Completely different body - changed
	diff := makeResponseData("secret hidden content", 200, 0)
	if !responseChangedIgnoringReflections(baselines, diff, false, 0.9) {
		t.Error("completely different body should be changed")
	}
}

func TestBaselineResponsesAreConsistent(t *testing.T) {
	// All same
	consistent := []ResponseData{
		makeResponseData("same", 200, 0),
		makeResponseData("same", 200, 0),
		makeResponseData("same", 200, 0),
	}
	if !baselineResponsesAreConsistent(consistent, responsesAreEqual) {
		t.Error("identical baselines should be consistent")
	}

	// One different
	inconsistent := []ResponseData{
		makeResponseData("same", 200, 0),
		makeResponseData("same", 200, 0),
		makeResponseData("different", 200, 0),
	}
	if baselineResponsesAreConsistent(inconsistent, responsesAreEqual) {
		t.Error("baselines with one different should not be consistent with equal check")
	}

	// Similar but not equal (dynamic page) - consistent with similarity check
	dynamic := []ResponseData{
		makeResponseData("page content timestamp=111 "+loremIpsum, 200, 0),
		makeResponseData("page content timestamp=222 "+loremIpsum, 200, 0),
		makeResponseData("page content timestamp=333 "+loremIpsum, 200, 0),
	}
	similarCheck := func(a, b ResponseData) bool {
		return responsesAreSimilar(a, b, 0.9)
	}
	if !baselineResponsesAreConsistent(dynamic, similarCheck) {
		t.Error("similar dynamic baselines should be consistent with similarity check")
	}

	// Single baseline is always consistent
	single := []ResponseData{makeResponseData("one", 200, 0)}
	if !baselineResponsesAreConsistent(single, responsesAreEqual) {
		t.Error("single baseline should always be consistent")
	}

	// Empty baselines
	if !baselineResponsesAreConsistent([]ResponseData{}, responsesAreEqual) {
		t.Error("empty baselines should be consistent")
	}
}
