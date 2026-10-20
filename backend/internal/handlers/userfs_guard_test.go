package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// fileFunctions are the os/filepath functions that touch the filesystem by
// path. Handlers reach user files through userfs instead (see CLAUDE.md).
var fileFunctions = map[string]map[string]bool{
	"os": {"Open": true, "OpenFile": true, "Create": true, "ReadFile": true, "WriteFile": true,
		"ReadDir": true, "Stat": true, "Lstat": true, "Mkdir": true, "MkdirAll": true, "Remove": true,
		"RemoveAll": true, "Rename": true, "Chmod": true, "Chtimes": true, "Symlink": true, "Link": true,
		"Truncate": true, "CreateTemp": true, "MkdirTemp": true},
	"filepath": {"Walk": true, "WalkDir": true},
}

// allowedFileAccess lists the handler functions that may use them, and why:
// they work on the app's own data (caches, temp files, new users' folders) or
// read at a real location already checked by userfs.RealPath.
var allowedFileAccess = map[string]string{
	"archive.go:openTar":                  "reads an archive at its checked real location",
	"cache.go:Clear":                      "the HLS and conversion caches",
	"conv_cache.go:newConvCache":          "the conversion cache",
	"conv_cache.go:get":                   "the conversion cache",
	"conv_cache.go:lookup":                "the conversion cache",
	"conv_cache.go:put":                   "the conversion cache",
	"conv_cache.go:cleanup":               "the conversion cache",
	"hls.go:Playlist":                     "the HLS cache",
	"hls.go:Segment":                      "the HLS cache",
	"hls.go:ensureGenerating":             "the HLS cache",
	"hls.go:fileExists":                   "the HLS cache",
	"hls.go:cleanupOldCache":              "the HLS cache",
	"advancedrenamer.go:collectFileVars":  "reads metadata at a checked real location",
	"advancedrenamer.go:collectImageVars": "reads metadata at a checked real location",
	"thumbnails.go:Get":                   "the thumbnail cache",
	"users.go:Create":                     "creates a new user's folder",
	"files.go:DownloadZip":                "a temp file",
	"files.go:convertAudioAndStream":      "the conversion cache and a temp file",
	"files.go:transcodeAudio":             "the conversion cache",
}

// TestHandlersReachUserFilesThroughUserfs fails when a handler function not
// listed above touches the filesystem by path (os.Open, os.Remove, …) or
// serves a file by path (c.File): that would bypass the folder checks.
func TestHandlersReachUserFilesThroughUserfs(t *testing.T) {
	sources, _ := filepath.Glob("*.go")
	var found []string
	used := map[string]bool{}
	for _, source := range sources {
		if strings.HasSuffix(source, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), source, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			key := source + ":" + function.Name.Name
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				receiver, _ := selector.X.(*ast.Ident)
				name := selector.Sel.Name
				byPath := receiver != nil && fileFunctions[receiver.Name][name]
				serves := name == "File" || name == "FileAttachment"
				if !byPath && !serves {
					return true
				}
				if _, allowed := allowedFileAccess[key]; allowed {
					used[key] = true
					return true
				}
				callName := name
				if receiver != nil {
					callName = receiver.Name + "." + name
				}
				found = append(found, key+": "+callName)
				return true
			})
		}
	}
	if len(found) > 0 {
		sort.Strings(found)
		t.Errorf("file access by path in handlers (use userfs, or list it in allowedFileAccess with a reason):\n%s", strings.Join(found, "\n"))
	}
	var stale []string
	for key := range allowedFileAccess {
		if !used[key] {
			stale = append(stale, key)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("allowedFileAccess lists functions that no longer access files (remove them):\n%s", strings.Join(stale, "\n"))
	}
}
