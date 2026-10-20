package diskusage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"files/internal/userfs"
)

// folder creates files (name → length) in a fresh folder and opens it.
func folder(t *testing.T, files map[string]int) (*userfs.FS, string) {
	t.Helper()
	dir := t.TempDir()
	for name, length := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Repeat("x", length)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fsys, err := userfs.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fsys.Close() })
	return fsys, dir
}

func scan(t *testing.T, fsys *userfs.FS, name string) *Result {
	t.Helper()
	result, err := Scan(context.Background(), fsys, name, nil)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func paths(files []File) []string {
	names := make([]string, len(files))
	for i, file := range files {
		names[i] = filepath.ToSlash(file.Path)
	}
	return names
}

func TestScanTotalsEveryFolder(t *testing.T) {
	fsys, _ := folder(t, map[string]int{"a/one.txt": 100, "a/b/two.txt": 300, "c.txt": 50, "empty/.keep": 0})
	result := scan(t, fsys, ".")

	root := result.Root
	if root.Files != 4 || root.Dirs != 3 || root.Apparent != 450 {
		t.Errorf("root = %+v, want 4 files, 3 folders, 450 bytes", root.Totals)
	}
	a := result.Find("a")
	if a == nil || a.Files != 2 || a.Dirs != 1 || a.Apparent != 400 {
		t.Fatalf("a = %+v", a)
	}
	if b := result.Find(filepath.Join("a", "b")); b == nil || b.Files != 1 || b.Dirs != 0 || b.Apparent != 300 {
		t.Errorf("a/b = %+v", b)
	}
	// Folders take blocks of their own, which only the disk size counts.
	if a.Disk <= result.Find(filepath.Join("a", "b")).Disk || root.Disk <= a.Disk {
		t.Errorf("disk sizes don't add up: root %d, a %d", root.Disk, a.Disk)
	}
	if result.ScannedAt.IsZero() {
		t.Error("no scan time")
	}

	for _, name := range []string{"c.txt", "missing", "..", filepath.Join("..", "a")} {
		if result.Find(name) != nil {
			t.Errorf("Find(%q) found a folder", name)
		}
	}
	if !result.Contains(filepath.Join("a", "one.txt")) || result.Contains("..") {
		t.Error("Contains is wrong about what's inside")
	}
}

func TestScanOfASubfolder(t *testing.T) {
	fsys, _ := folder(t, map[string]int{"a/one.txt": 100, "a/b/two.txt": 300, "c.txt": 50})
	result := scan(t, fsys, "a")
	if result.Root.Apparent != 400 || result.Root.Files != 2 {
		t.Errorf("root = %+v", result.Root.Totals)
	}
	if result.Find("b") != nil || result.Find(filepath.Join("a", "b")) == nil || result.Find(".") != nil {
		t.Error("names aren't relative to the user's folder")
	}
	if result.Find("a") != result.Root {
		t.Error("the scanned folder isn't found by its own name")
	}
}

func TestScanRefusesWhatIsntAFolder(t *testing.T) {
	fsys, _ := folder(t, map[string]int{"c.txt": 50})
	if _, err := Scan(context.Background(), fsys, "c.txt", nil); !errors.Is(err, ErrNotFolder) {
		t.Errorf("file: err = %v", err)
	}
	if _, err := Scan(context.Background(), fsys, "missing", nil); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing: err = %v", err)
	}
	if _, err := Scan(context.Background(), fsys, "..", nil); err == nil {
		t.Error("scanned outside the folder")
	}
}

func TestHardLinksTakeSpaceOnce(t *testing.T) {
	fsys, dir := folder(t, map[string]int{"one/big.bin": 64 << 10, "two/big.bin": 64 << 10})
	if err := os.Link(filepath.Join(dir, "two", "big.bin"), filepath.Join(dir, "two", "again.bin")); err != nil {
		t.Skip("no hard links here:", err)
	}
	info, err := os.Lstat(filepath.Join(dir, "two", "again.bin"))
	if err != nil {
		t.Fatal(err)
	}
	folderInfo, _ := os.Lstat(filepath.Join(dir, "two"))
	if HardLinks(info) != 2 || HardLinks(folderInfo) != 1 {
		t.Errorf("hard links: file %d, folder %d", HardLinks(info), HardLinks(folderInfo))
	}
	one, two := scan(t, fsys, "one"), scan(t, fsys, "two")
	if two.Root.Files != 2 || two.Root.Apparent != 2*one.Root.Apparent {
		t.Errorf("apparent: %+v, one link %+v", two.Root.Totals, one.Root.Totals)
	}
	if two.Root.Disk != one.Root.Disk {
		t.Errorf("disk = %d, want %d (counted once)", two.Root.Disk, one.Root.Disk)
	}

	var counted Totals
	if err := Count(context.Background(), fsys, "two", &counted, func() {}); err != nil {
		t.Fatal(err)
	}
	if counted != two.Root.Totals {
		t.Errorf("Count = %+v, Scan = %+v", counted, two.Root.Totals)
	}
}

func TestLinksAreNotFollowed(t *testing.T) {
	fsys, dir := folder(t, map[string]int{"big/data.bin": 1 << 20, "sub/small.txt": 1})
	for target, link := range map[string]string{"../big": "sub/to-folder", "../big/data.bin": "sub/to-file"} {
		if err := os.Symlink(target, filepath.Join(dir, link)); err != nil {
			t.Fatal(err)
		}
	}
	result := scan(t, fsys, "sub")
	want := int64(1 + len("../big") + len("../big/data.bin"))
	if result.Root.Apparent != want || result.Root.Files != 3 || result.Root.Dirs != 0 {
		t.Errorf("root = %+v, want links counted as themselves (%d bytes)", result.Root.Totals, want)
	}
}

func TestLargestFiles(t *testing.T) {
	// A block or more apart, so they differ on disk too.
	fsys, dir := folder(t, map[string]int{"a/small.bin": 100, "a/b/big.bin": 300_000, "mid.bin": 60_000})
	if err := os.Link(filepath.Join(dir, "a", "b", "big.bin"), filepath.Join(dir, "z-big-again.bin")); err != nil {
		t.Fatal(err)
	}
	result := scan(t, fsys, ".")
	largest := result.Largest()
	want := []string{"a/b/big.bin", "mid.bin", "a/small.bin", "z-big-again.bin"}
	if got := paths(largest); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("largest = %v, want %v (a second name for a file takes no more space)", got, want)
	}
	if largest[0].Apparent != 300_000 || largest[3].Disk != 0 {
		t.Errorf("sizes = %+v", largest)
	}
}

func TestLargestFilesKeepsTheLargest(t *testing.T) {
	var kept fileHeap
	for size := 1; size <= LargestKept+5; size++ {
		kept.keep(File{Path: fmt.Sprintf("f%03d", size%7), Disk: int64(size * 4096)})
	}
	largest := kept.sorted()
	if len(largest) != LargestKept {
		t.Fatalf("kept %d files, want %d", len(largest), LargestKept)
	}
	if largest[0].Disk != (LargestKept+5)*4096 || largest[len(largest)-1].Disk != 6*4096 {
		t.Errorf("range %d..%d, want the largest %d", largest[0].Disk, largest[len(largest)-1].Disk, LargestKept)
	}
	for i := 1; i < len(largest); i++ {
		if largest[i].Disk > largest[i-1].Disk {
			t.Fatalf("not largest first at %d", i)
		}
	}

	var tied fileHeap
	for _, path := range []string{"c", "a", "b"} {
		tied.keep(File{Path: path, Disk: 4096})
	}
	if got := paths(tied.sorted()); strings.Join(got, ",") != "a,b,c" {
		t.Errorf("equal sizes = %v, want by path", got)
	}
}

func TestScanStopsWhenCancelled(t *testing.T) {
	fsys, _ := folder(t, map[string]int{"a/one.txt": 1, "a/two.txt": 2})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Scan(ctx, fsys, ".", nil); !errors.Is(err, context.Canceled) {
		t.Errorf("Scan: err = %v", err)
	}
	var counted Totals
	Count(ctx, fsys, ".", &counted, func() {})
	if counted.Files != 0 {
		t.Errorf("Count went on: %+v", counted)
	}
}

func TestScanReportsProgress(t *testing.T) {
	defer func(interval time.Duration) { progressInterval = interval }(progressInterval)
	progressInterval = 0
	fsys, _ := folder(t, map[string]int{"a/one.txt": 1, "a/two.txt": 2, "three.txt": 3})
	var reports []Totals
	result, err := Scan(context.Background(), fsys, ".", func(totals Totals) { reports = append(reports, totals) })
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) == 0 || reports[len(reports)-1] != result.Root.Totals {
		t.Errorf("reports %+v, final %+v", reports, result.Root.Totals)
	}
	for i := 1; i < len(reports); i++ {
		if reports[i].Files < reports[i-1].Files {
			t.Errorf("progress went back: %+v", reports)
		}
	}
}

func TestRemoveUpdatesTheScan(t *testing.T) {
	fsys, dir := folder(t, map[string]int{"a/one.txt": 100, "a/b/two.txt": 300, "a/b/three.txt": 200, "c.txt": 50})
	result := scan(t, fsys, ".")
	before := result.Root.Totals
	two := filepath.Join("a", "b", "two.txt")

	info, err := os.Lstat(filepath.Join(dir, two))
	if err != nil {
		t.Fatal(err)
	}
	removed := result.Remove(two, info)
	if removed.Files != 1 || removed.Apparent != 300 || removed.Disk != Allocated(info) || removed.Disk <= 0 {
		t.Errorf("removed %+v", removed)
	}
	if b := result.Find(filepath.Join("a", "b")); b.Apparent != 200 || b.Files != 1 {
		t.Errorf("a/b = %+v", b.Totals)
	}
	if a := result.Find("a"); a.Apparent != 300 || a.Files != 2 {
		t.Errorf("a = %+v", a.Totals)
	}
	if result.Root.Apparent != before.Apparent-300 || result.Root.Disk != before.Disk-removed.Disk {
		t.Errorf("root = %+v, before %+v", result.Root.Totals, before)
	}
	for _, file := range result.Largest() {
		if file.Path == two {
			t.Error("a removed file is still among the largest")
		}
	}

	aInfo, _ := os.Lstat(filepath.Join(dir, "a"))
	threeInfo, _ := os.Lstat(filepath.Join(dir, "a", "b", "three.txt"))
	removed = result.Remove("a", aInfo)
	if removed.Dirs != 2 || removed.Files != 2 || removed.Apparent != 300 {
		t.Errorf("removed folder %+v", removed)
	}
	if result.Find("a") != nil || result.Root.Dirs != 0 || result.Root.Files != 1 || result.Root.Apparent != 50 {
		t.Errorf("after removing a: root %+v", result.Root.Totals)
	}
	// Deleted together with a folder above it: nothing more to take off.
	if removed := result.Remove(filepath.Join("a", "b", "three.txt"), threeInfo); removed != (Totals{}) {
		t.Errorf("a file in a removed folder took off %+v", removed)
	}
	if got := paths(result.Largest()); len(got) != 1 || got[0] != "c.txt" {
		t.Errorf("largest after removing a = %v", got)
	}

	rootInfo, _ := os.Lstat(dir)
	if result.Remove(".", rootInfo) != (Totals{}) || result.Remove("..", rootInfo) != (Totals{}) || result.Remove("missing", rootInfo) != (Totals{}) {
		t.Error("removed the scanned folder, something outside it, or a folder it doesn't have")
	}
	if result.Root.Files != 1 {
		t.Errorf("root changed: %+v", result.Root.Totals)
	}
}

func TestUnreadableFoldersAreCounted(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads every folder")
	}
	fsys, dir := folder(t, map[string]int{"locked/secret.txt": 10, "open.txt": 5})
	locked := filepath.Join(dir, "locked")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(locked, 0o755)
	result := scan(t, fsys, ".")
	if result.Unreadable != 1 || result.Root.Apparent != 5 || result.Find("locked") == nil {
		t.Errorf("unreadable %d, root %+v", result.Unreadable, result.Root.Totals)
	}
}
