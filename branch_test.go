package main

// Tests for the branch model: ordering, the age column, the row layout and
// the commands the picker runs.
//
// The expected strings come from the Python picker's own output, so the two
// implementations are held to the same behaviour.

import (
	"reflect"
	"testing"
)

const testNow = int64(1795000000)

func TestPinnedBranchesLeadInPinnedOrder(t *testing.T) {
	branches := []Branch{
		{Name: "zeta"},
		{Name: "production"},
		{Name: "feature/x"},
		{Name: "master"},
		{Name: "main"},
		{Name: "uat"},
	}
	sortBranches(branches, sortTime)

	want := []string{"main", "master", "uat", "production", "feature/x", "zeta"}
	got := make([]string, 0, len(branches))
	for _, b := range branches {
		got = append(got, b.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pinned order: got %v, want %v", got, want)
	}
}

func TestPinningIsCaseInsensitive(t *testing.T) {
	// PROD, Prod and prod all take the same slot rather than only the
	// lowercase spelling being recognised.
	branches := []Branch{
		{Name: "PROD"},
		{Name: "prod"},
		{Name: "Prod"},
		{Name: "aaa"},
	}
	sortBranches(branches, sortTime)

	if branches[0].Name != "PROD" {
		t.Fatalf("a case variant of a pinned name should lead, got %q", branches[0].Name)
	}
}

func TestNearMissNamesAreNotPinned(t *testing.T) {
	// prods, uat-2 and preprod are not pins, even though they start with
	// the same letters as ones that are.
	branches := []Branch{
		{Name: "prods"},
		{Name: "uat-2"},
		{Name: "preprod"},
		{Name: "main"},
	}
	sortBranches(branches, sortTime)

	want := []string{"main", "preprod", "prods", "uat-2"}
	got := make([]string, 0, len(branches))
	for _, b := range branches {
		got = append(got, b.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("near misses: got %v, want %v", got, want)
	}
}

func TestPinsLeadInNameModeToo(t *testing.T) {
	branches := []Branch{
		{Name: "aaa"},
		{Name: "main"},
		{Name: "zzz"},
	}
	sortBranches(branches, sortName)

	want := []string{"main", "aaa", "zzz"}
	got := make([]string, 0, len(branches))
	for _, b := range branches {
		got = append(got, b.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("name mode: got %v, want %v", got, want)
	}
}

func TestTimeModeSortsNewestFirst(t *testing.T) {
	branches := []Branch{
		{Name: "old", Timestamp: 100},
		{Name: "new", Timestamp: 900},
		{Name: "middle", Timestamp: 500},
	}
	sortBranches(branches, sortTime)

	want := []string{"new", "middle", "old"}
	got := make([]string, 0, len(branches))
	for _, b := range branches {
		got = append(got, b.Name)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("time mode: got %v, want %v", got, want)
	}
}

func TestFormatAge(t *testing.T) {
	cases := []struct {
		age  int64
		want string
	}{
		{15, "15s"},
		{59, "59s"},
		{60, "1m"},
		{3599, "59m"},
		{3600, "1h"},
		{86399, "23h"},
		{86400, "1d"},
		{2591999, "29d"},
		{2592000, "1mo"},
		{31535999, "12mo"},
		{31536000, "1y"},
	}
	for _, c := range cases {
		if got := formatAge(testNow-c.age, testNow); got != c.want {
			t.Errorf("formatAge(%d): got %q, want %q", c.age, got, c.want)
		}
	}
}

func TestFormatAgeWithoutATimestamp(t *testing.T) {
	if got := formatAge(0, testNow); got != "?" {
		t.Fatalf("a branch with no date: got %q, want %q", got, "?")
	}
}

func TestDisplayLabel(t *testing.T) {
	cases := []struct {
		name    string
		branch  Branch
		current string
		want    string
	}{
		{
			name:    "current branch is marked",
			branch:  Branch{Name: "master", Timestamp: testNow - 172800, Upstream: "origin/master"},
			current: "master",
			want:    "* master                  2d",
		},
		{
			name:    "remote branch carries the tag",
			branch:  Branch{Name: "feature/x", Remote: true, Timestamp: testNow - 60},
			current: "master",
			want:    "  feature/x   [remote]    1m",
		},
		{
			name:    "plain branch has neither",
			branch:  Branch{Name: "zz", Timestamp: testNow - 900},
			current: "master",
			want:    "  zz                     15m",
		},
	}
	for _, c := range cases {
		if got := displayLabel(c.branch, c.current, testNow, 10); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSwitchAndPullCommands(t *testing.T) {
	local := Branch{Name: "master", Upstream: "origin/master"}
	remote := Branch{Name: "feature/x", Remote: true}

	if got := switchCommand(local); got != "git switch master" {
		t.Errorf("switch command: got %q", got)
	}
	if got := switchCommand(remote); got != "git switch --track origin/feature/x" {
		t.Errorf("remote switch command: got %q", got)
	}
	if got := pullCommand(local); got != "git pull" {
		t.Errorf("pull command: got %q", got)
	}
	if got := pullCommand(remote); got != "git pull" {
		t.Errorf("remote pull command: got %q", got)
	}
	if got := pullCommand(Branch{Name: "lonely"}); got != "git pull  # fails, no upstream" {
		t.Errorf("pull command with no upstream: got %q", got)
	}
}

func TestFitWidthMarksTruncation(t *testing.T) {
	if got := fitWidth("git switch --track origin/feature/x", 20); got != "git switch --trac..." {
		t.Errorf("truncated: got %q", got)
	}
	if got := fitWidth("git switch master", 20); got != "git switch master" {
		t.Errorf("short enough is untouched: got %q", got)
	}
}

func TestRepoLabel(t *testing.T) {
	cases := []struct {
		name string
		root string
		want string
	}{
		{"a normal folder", `C:\repo\git-picker`, "git-picker"},
		{"a trailing separator is ignored", `C:\repo\git-picker\`, "git-picker"},
		{"a drive root falls back to the path", `C:\`, `C:\`},
	}
	for _, c := range cases {
		if got := repoLabel(c.root); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestNtBase(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{`C:\`, ""},
		{"C:", ""},
		{`C:\repo\git-picker`, "git-picker"},
		{`C:\repo\git-picker\`, ""},
		{"C:/repo/git-picker", "git-picker"},
		{"/repo/git-picker", "git-picker"},
		{"repo/git-picker", "git-picker"},
	}
	for _, c := range cases {
		if got := ntBase(c.path); got != c.want {
			t.Errorf("ntBase(%q): got %q, want %q", c.path, got, c.want)
		}
	}
}

func TestPaddedColumnsKeepTheirWidth(t *testing.T) {
	// The columns are aligned by cell count, not by rune count.
	if got := displayWidth(padCells("master", 10)); got != 10 {
		t.Errorf("padCells width: got %d", got)
	}
	if got := displayWidth(rjustCells("2d", 4)); got != 4 {
		t.Errorf("rjustCells width: got %d", got)
	}
}
