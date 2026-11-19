package main

// Tests for the fuzzy matcher.
//
// The scorer is a port of pfzy's fzy_scorer, which is what InquirerPy's
// fuzzy prompt uses by default. The expected scores are the ones pfzy
// itself produces, so a change in the port shows up as a changed score
// rather than as a differently ordered list.

import (
	"math"
	"testing"
)

func TestFzyScorerMatchesPfzy(t *testing.T) {
	cases := []struct {
		needle   string
		haystack string
		score    float64
	}{
		{"ab", "acb", 0.89},
		{"ab", "acbabc", 0.98},
		{"ab", "wc", math.Inf(-1)},
	}
	for _, c := range cases {
		score, indices := fzyScorer(c.needle, c.haystack)
		if c.score == math.Inf(-1) {
			if indices != nil {
				t.Errorf("fzyScorer(%q, %q): expected no match, got indices %v", c.needle, c.haystack, indices)
			}
			continue
		}
		if math.Abs(score-c.score) > 1e-9 {
			t.Errorf("fzyScorer(%q, %q): got score %v, want %v", c.needle, c.haystack, score, c.score)
		}
	}
}

func TestFzyScorerIndices(t *testing.T) {
	score, indices := fzyScorer("ab", "acbabc")
	if score < 0 {
		t.Fatalf("expected a match, got score %v", score)
	}
	if len(indices) != 2 || indices[0] != 3 || indices[1] != 4 {
		t.Fatalf("indices: got %v, want [3 4]", indices)
	}
}

func TestNoMatchReturnsNoIndices(t *testing.T) {
	if _, indices := fzyScorer("xyzzy", "master"); indices != nil {
		t.Fatalf("expected no match, got %v", indices)
	}
}

func TestEmptyNeedleMatchesEverything(t *testing.T) {
	if !isSubsequence("", "master") {
		t.Fatal("an empty needle should match anything")
	}
}

func TestSubsequenceIsCaseInsensitive(t *testing.T) {
	if !isSubsequence("MASTER", "master") {
		t.Fatal("the subsequence check should ignore case")
	}
	if !isSubsequence("mt", "master") {
		t.Fatal("m then t is a subsequence of master")
	}
	if isSubsequence("tm", "master") {
		t.Fatal("t then m is not a subsequence of master")
	}
}

func TestIsLower(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"master", true},
		{"Master", false},
		{"123", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isLower(c.text); got != c.want {
			t.Errorf("isLower(%q): got %v, want %v", c.text, got, c.want)
		}
	}
}

func TestFilterBranchesKeepsOrderForAnEmptyQuery(t *testing.T) {
	branches := []Branch{{Name: "b"}, {Name: "a"}, {Name: "c"}}
	labels := []string{"  b", "  a", "  c"}

	matches := filterBranches("", branches, labels)
	if len(matches) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matches))
	}
	for i, entry := range matches {
		if branches[entry.index].Name != []string{"b", "a", "c"}[i] {
			t.Fatalf("order changed at %d: got %q", i, branches[entry.index].Name)
		}
	}
}

func TestFilterBranchesDropsNonMatches(t *testing.T) {
	branches := []Branch{{Name: "master"}, {Name: "feature/x"}}
	labels := []string{"  master", "  feature/x"}

	matches := filterBranches("master", branches, labels)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if branches[matches[0].index].Name != "master" {
		t.Fatalf("wrong match: %q", branches[matches[0].index].Name)
	}
}

func TestFilterBranchesRanksHigherScoresFirst(t *testing.T) {
	branches := []Branch{{Name: "a-master"}, {Name: "master"}}
	labels := []string{"  a-master", "  master"}

	matches := filterBranches("master", branches, labels)
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if branches[matches[0].index].Name != "master" {
		t.Fatalf("the better match should come first, got %q", branches[matches[0].index].Name)
	}
}
