package loadshape

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Golden grid parameters (RFC-0001 D5, ADR-0003): a fixed absolute
// timestamp grid so the parity goldens are deterministic and reviewable.
// ref = 2026-01-05T00:00:00Z (a Monday), the window spans the 14 days
// up to and including ref (seeded history is generated backwards from a
// reference time) at 30-minute steps, chosen so it exercises every
// weekday, at least one full diurnal cycle, and -- because the three
// profile.json anomalies are pinned at offset_days -10, -6, -2 -- all
// three anomalies.
const (
	goldenRefUnix    = 1767571200 // 2026-01-05T00:00:00Z
	goldenStepSecs   = 1800       // 30 minutes
	goldenWindowDays = 14
)

// repoRoot locates the repository root from this test file's own path,
// so the parity check works regardless of the working directory `go
// test` is invoked from (Hard rule 8: profile.json and the goldens are
// read from their one checked-in location, not copied into the module).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed to resolve parity_test.go path")
	}
	// services/analytics/internal/loadshape/parity_test.go -> repo root
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "..", ".."))
}

func loadProfile(t *testing.T, root string) Profile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "loadprofile", "profile.json"))
	if err != nil {
		t.Fatalf("read profile.json: %v", err)
	}
	p, err := ParseProfile(data)
	if err != nil {
		t.Fatalf("parse profile.json: %v", err)
	}
	return p
}

func goldenGrid() []int64 {
	start := int64(goldenRefUnix - goldenWindowDays*secondsPerDay)
	end := int64(goldenRefUnix)
	grid := make([]int64, 0, (end-start)/goldenStepSecs+1)
	for ts := start; ts <= end; ts += goldenStepSecs {
		grid = append(grid, ts)
	}
	return grid
}

func goldenRows(profile Profile, scale float64) []string {
	rows := make([]string, 0, len(goldenGrid())+1)
	rows = append(rows, "unix_ts,rate")
	for _, ts := range goldenGrid() {
		r := Rate(float64(ts), profile, goldenRefUnix, scale)
		rows = append(rows, fmt.Sprintf("%d,%.9f", ts, r))
	}
	return rows
}

// TestGenerateGoldens (re)writes loadprofile/parity/golden-scale{1,24}.csv
// from this package's Rate implementation -- Go is the canonical impl
// (team-plan.md); the JS side (loadprofile/parity/compare.mjs) only
// ever reads these files, never regenerates them. Guarded by
// REGEN_GOLDENS=1 so a normal `go test ./...` never rewrites goldens
// unnoticed -- regeneration is a deliberate, reviewed step (see
// loadprofile/README.md).
func TestGenerateGoldens(t *testing.T) {
	if os.Getenv("REGEN_GOLDENS") != "1" {
		t.Skip("set REGEN_GOLDENS=1 to regenerate loadprofile/parity goldens")
	}
	root := repoRoot(t)
	profile := loadProfile(t, root)

	for _, tc := range []struct {
		scale float64
		file  string
	}{
		{1, "golden-scale1.csv"},
		{24, "golden-scale24.csv"},
	} {
		path := filepath.Join(root, "loadprofile", "parity", tc.file)
		content := strings.Join(goldenRows(profile, tc.scale), "\n") + "\n"
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
}

// TestParityGoldens is the parity gate (Hard rule 8): it recomputes the
// golden grid from this package's Rate and compares it, byte-for-byte
// against the checked-in CSVs, at both scales. Go is canonical, so this
// must match its own goldens exactly -- any mismatch means the goldens
// are stale (profile.json or loadshape.go changed without regenerating
// them via REGEN_GOLDENS=1) which is exactly the drift Hard rule 8
// forbids.
func TestParityGoldens(t *testing.T) {
	root := repoRoot(t)
	profile := loadProfile(t, root)

	for _, tc := range []struct {
		scale float64
		file  string
	}{
		{1, "golden-scale1.csv"},
		{24, "golden-scale24.csv"},
	} {
		path := filepath.Join(root, "loadprofile", "parity", tc.file)
		want := goldenRows(profile, tc.scale)
		got := readCSVLines(t, path)
		if len(want) != len(got) {
			t.Fatalf("%s: row count mismatch: computed %d, golden %d", tc.file, len(want), len(got))
		}
		for i := range want {
			if want[i] != got[i] {
				t.Fatalf("%s: row %d mismatch:\n  computed: %s\n  golden:   %s", tc.file, i, want[i], got[i])
			}
		}
	}
}

func readCSVLines(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s: %v", path, err)
	}
	return lines
}

// TestGoldenGridSanity guards the grid parameters themselves: if
// goldenRefUnix ever moves, the anomaly offsets in profile.json (-10,
// -6, -2 days) must still land inside [ref-14d, ref] or the parity
// goldens stop exercising all three story anomalies (see loadprofile/README.md).
func TestGoldenGridSanity(t *testing.T) {
	grid := goldenGrid()
	first, last := grid[0], grid[len(grid)-1]
	root := repoRoot(t)
	profile := loadProfile(t, root)
	for _, a := range profile.Anomalies {
		start := int64(goldenRefUnix + a.OffsetDays*secondsPerDay)
		end := start + int64(a.DurationHours*secondsPerHour)
		if start < first || end > last {
			t.Fatalf("anomaly %q window [%d,%d] falls outside golden grid [%d,%d]", a.Name, start, end, first, last)
		}
	}
}
