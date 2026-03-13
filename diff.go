package main

import "github.com/sergi/go-diff/diffmatchpatch"

func responseChanged(baselineResponses []ResponseData, new ResponseData, equalCheck bool, threshold float64) bool {
	for _, baseline := range baselineResponses {
		if equalCheck && responsesAreEqual(baseline, new) {
			return false
		} else if !equalCheck && responsesAreSimilar(baseline, new, threshold) {
			return false
		}
	}
	return true // Response is different from all baselines; significant change detected
}

// responseChangedIgnoringReflections checks if the response changed compared to
// baselines, but ignores the Reflections field. Used for testing form parameters
// that are already present in baseline HTML to avoid false positives.
func responseChangedIgnoringReflections(baselineResponses []ResponseData, new ResponseData, sameBody bool, threshold float64) bool {
	for _, baseline := range baselineResponses {
		if baseline.StatusCode != new.StatusCode {
			continue
		}
		if sameBody {
			if baseline.BodyHash == new.BodyHash {
				return false
			}
		} else {
			if baseline.BodyHash == new.BodyHash {
				return false
			}
			similarity := computeSimilarity(baseline.Body, new.Body)
			if similarity >= threshold {
				return false
			}
		}
	}
	return true
}

func responsesAreSimilar(a, b ResponseData, threshold float64) bool {
	similarity := 1.0
	if a.BodyHash != b.BodyHash {
		similarity = computeSimilarity(a.Body, b.Body)
	}

	return a.StatusCode == b.StatusCode &&
		a.Reflections == b.Reflections &&
		similarity >= threshold
}

func responsesAreEqual(a, b ResponseData) bool {
	return a.StatusCode == b.StatusCode &&
		a.Reflections == b.Reflections &&
		a.BodyHash == b.BodyHash
}

func baselineResponsesAreConsistent(baselineResponses []ResponseData, compareFunc func(ResponseData, ResponseData) bool) bool {
	for i := 0; i < len(baselineResponses); i++ {
		for j := i + 1; j < len(baselineResponses); j++ {
			if !compareFunc(baselineResponses[i], baselineResponses[j]) {
				return false
			}
		}
	}
	return true
}

func computeSimilarity(aBody, bBody []byte) float64 {
	aText := string(aBody)
	bText := string(bBody)

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(aText, bText, false)

	distance := dmp.DiffLevenshtein(diffs)

	// Calculate the maximum possible distance
	maxLen := len(aText)
	if len(bText) > maxLen {
		maxLen = len(bText)
	}

	if maxLen == 0 {
		return 1.0 // Both strings are empty, so they are identical
	}

	// Compute similarity as (1 - (distance / maxLen))
	similarity := 1 - float64(distance)/float64(maxLen)
	return similarity
}
