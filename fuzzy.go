package main

// A port of pfzy's fzy_scorer, which is the scorer InquirerPy's fuzzy prompt
// uses by default (match_exact defaults to False, so the default is fzy, not
// substr). Every constant and the shape of the dynamic program come from
// pfzy/score.py, so the Go picker filters and ranks a query exactly the way
// the Python one does.

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

const (
	scoreGapLeading      = -0.005
	scoreGapTrailing     = -0.005
	scoreGapInner        = -0.01
	scoreMatchConsecutiv = 1.0

	scoreMatchSlash   = 0.9
	scoreMatchWord    = 0.8
	scoreMatchCapital = 0.7
	scoreMatchDot     = 0.6
)

// bonusFor returns the bonus the previous character awards to the character
// at the given index, following pfzy's BONUS_STATES table:
//
//	lowercase, digit -> BONUS_MAP: bonus only after a word separator
//	uppercase        -> BONUS_MAP plus 0.7 after any lowercase letter
//	anything else    -> no bonus at all
//
// The previous character starts as '/', so a leading '/' earns the slash
// bonus for the first character.
func bonusFor(prev rune, cur rune) float64 {
	switch {
	case cur >= 'a' && cur <= 'z', cur >= '0' && cur <= '9':
		return separatorBonus(prev)
	case cur >= 'A' && cur <= 'Z':
		if v := separatorBonus(prev); v != 0 {
			return v
		}
		if prev >= 'a' && prev <= 'z' {
			return scoreMatchCapital
		}
		return 0
	}
	return 0
}

// separatorBonus is the bonus awarded when the previous character marks the
// start of a new word.
func separatorBonus(prev rune) float64 {
	switch prev {
	case '/':
		return scoreMatchSlash
	case '-', '_', ' ':
		return scoreMatchWord
	case '.':
		return scoreMatchDot
	}
	return 0
}

// bonusScores is pfzy's _bonus: one bonus per character of the haystack.
func bonusScores(haystack []rune) []float64 {
	bonus := make([]float64, len(haystack))
	prev := '/'
	for i, r := range haystack {
		bonus[i] = bonusFor(prev, r)
		prev = r
	}
	return bonus
}

// isLower is Python's str.islower: true when there is at least one cased
// character and every cased character is lowercase.
func isLower(s string) bool {
	hasCased := false
	for _, r := range s {
		switch {
		case unicode.IsUpper(r), unicode.IsTitle(r):
			return false
		case unicode.IsLower(r):
			hasCased = true
		}
	}
	return hasCased
}

// isSubsequence is pfzy's _subsequence: every character of the needle must
// appear in the haystack in order.
func isSubsequence(needle, haystack string) bool {
	needle = strings.ToLower(needle)
	haystack = strings.ToLower(haystack)
	if needle == "" {
		return true
	}
	offset := 0
	for _, r := range needle {
		found := strings.IndexRune(haystack[offset:], r)
		if found < 0 {
			return false
		}
		offset += found + len(string(r))
	}
	return true
}

// fzyScore is pfzy's _score: the dynamic program that finds the best
// subsequence match and its score.
//
// Returns the score and the matched character indices. The second result is
// nil when the needle is not a subsequence of the haystack at all.
func fzyScore(needle, haystack string) (float64, []int) {
	needleRunes := []rune(needle)
	haystackRunes := []rune(haystack)

	bonus := bonusScores(haystackRunes)

	// Smart case: a lowercase needle matches a lowercase haystack too.
	if isLower(needle) {
		haystackRunes = []rune(strings.ToLower(haystack))
	}

	needleLen, haystackLen := len(needleRunes), len(haystackRunes)

	if needleLen == 0 || needleLen == haystackLen {
		indices := make([]int, needleLen)
		for i := range indices {
			indices[i] = i
		}
		return math.Inf(1), indices
	}

	negInf := math.Inf(-1)

	// running[i][j] is the best score ending at a match of needle[i] and
	// haystack[j]. result[i][j] is the best score for needle[:i+1] within
	// haystack[:j+1].
	running := make([][]float64, needleLen)
	result := make([][]float64, needleLen)
	for i := range running {
		running[i] = make([]float64, haystackLen)
		result[i] = make([]float64, haystackLen)
		for j := range running[i] {
			running[i][j] = negInf
		}
	}

	for i := 0; i < needleLen; i++ {
		prevScore := negInf
		gap := scoreGapInner
		if i == needleLen-1 {
			gap = scoreGapTrailing
		}
		for j := 0; j < haystackLen; j++ {
			if needleRunes[i] == haystackRunes[j] {
				score := negInf
				if i == 0 {
					score = float64(j)*scoreGapLeading + bonus[j]
				} else if j != 0 {
					// A consecutive match beats a match that skipped
					// characters, so take whichever is higher.
					consecutive := running[i-1][j-1] + scoreMatchConsecutiv
					skipped := result[i-1][j-1] + bonus[j]
					if consecutive > skipped {
						score = consecutive
					} else {
						score = skipped
					}
				}
				running[i][j] = score
				if score > prevScore+gap {
					prevScore = score
				} else {
					prevScore = prevScore + gap
				}
				result[i][j] = prevScore
			} else {
				running[i][j] = negInf
				prevScore = prevScore + gap
				result[i][j] = prevScore
			}
		}
	}

	// Backtrack for the indices of the optimal match, preferring the first
	// match encountered working backwards, as pfzy does.
	indices := make([]int, needleLen)
	i, j := needleLen-1, haystackLen-1
	matchRequired := false
	for i >= 0 {
		for j >= 0 {
			if (matchRequired || running[i][j] == result[i][j]) && running[i][j] != negInf {
				matchRequired = i > 0 && j > 0 &&
					result[i][j] == running[i-1][j-1]+scoreMatchConsecutiv
				indices[i] = j
				j--
				break
			}
			j--
		}
		i--
	}

	return result[needleLen-1][haystackLen-1], indices
}

// fzyScorer is pfzy's fzy_scorer: -inf and no indices when the needle is not
// a subsequence of the haystack, the fzy score otherwise.
func fzyScorer(needle, haystack string) (float64, []int) {
	if !isSubsequence(needle, haystack) {
		return math.Inf(-1), nil
	}
	return fzyScore(needle, haystack)
}

// match is one filtered branch: where it sits in the branch list plus where
// the query hit. The index is used rather than a pointer because the branch
// slice is re-sorted in place when the sort mode toggles.
type match struct {
	index   int
	score   float64
	indices []int
}

// filterBranches applies a query to the branches and returns the matches in
// rank order.
//
// An empty query matches everything and keeps the given order. Otherwise the
// query is scored against each branch's display label, branches that do not
// match are dropped, and the rest are ranked by score, highest first, with
// ties keeping their original order. That is pfzy's fuzzy_match.
func filterBranches(query string, branches []Branch, labels []string) []match {
	if query == "" {
		out := make([]match, 0, len(branches))
		for i := range branches {
			out = append(out, match{index: i, score: 0, indices: nil})
		}
		return out
	}

	var out []match
	for i := range branches {
		score, indices := fzyScorer(query, labels[i])
		if indices == nil {
			continue
		}
		out = append(out, match{index: i, score: score, indices: indices})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	return out
}
