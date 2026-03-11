package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/rwcarlsen/goexif/exif"
)

// ── Handler ───────────────────────────────────────────────────────────────────

type AdvancedRenamerHandler struct {
	db *sql.DB
}

func NewAdvancedRenamerHandler(db *sql.DB) *AdvancedRenamerHandler {
	return &AdvancedRenamerHandler{db: db}
}

// ── Request / response types ──────────────────────────────────────────────────

type renamePreviewRequest struct {
	Template string   `json:"template"`
	Paths    []string `json:"paths"`
}

type renamePreviewItem struct {
	OriginalName string `json:"original_name"`
	NewName      string `json:"new_name"`
	Error        string `json:"error,omitempty"`
}

type renameApplyOp struct {
	From string `json:"from"` // relative path
	To   string `json:"to"`   // new filename only
}

type renameApplyRequest struct {
	Renames      []renameApplyOp `json:"renames"`
	Folder       string          `json:"folder"`
	Template     string          `json:"template"`
	ConflictMode string          `json:"conflict_mode"` // "error" | "replace" | "rename"
}

type renamePresetRow struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Template  string `json:"template"`
	CreatedAt string `json:"created_at"`
}

// ── Template engine ───────────────────────────────────────────────────────────

var templateTokenRe = regexp.MustCompile(`\{([^}]+)\}`)

// allKnownVars is the complete set of variable names the engine understands.
// Variables not in this set produce an error token in the output.
var allKnownVars = func() map[string]bool {
	names := []string{
		"name", "ext", "index", "size", "random",
		"created_date", "created_time", "modified_date", "modified_time",
		"width", "height", "megapixels", "camera", "iso", "aperture", "shutter",
		"duration", "fps", "resolution", "vcodec",
		"artist", "album", "title", "track", "year",
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set
}()

type templateVars map[string]string

// renderTemplate substitutes {variable} and {variable|t1|t2} tokens in the
// template string using the provided variable map. Returns the rendered string
// and a list of unknown variable names (for error display).
func renderTemplate(tmpl string, vars templateVars) (string, []string) {
	var unknowns []string
	result := templateTokenRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		inner := match[1 : len(match)-1]
		parts := strings.Split(inner, "|")
		varName := strings.TrimSpace(parts[0])
		transforms := parts[1:]

		if !allKnownVars[varName] {
			unknowns = append(unknowns, varName)
			return "⟨" + varName + "?⟩"
		}

		value := vars[varName]
		for _, transform := range transforms {
			value = applyTransform(value, strings.TrimSpace(transform))
		}
		return value
	})
	return result, unknowns
}

func applyTransform(value, transform string) string {
	name, arg, _ := strings.Cut(transform, ":")
	switch name {
	case "upper":
		return strings.ToUpper(value)
	case "lower":
		return strings.ToLower(value)
	case "title":
		words := strings.Fields(value)
		for i, word := range words {
			if len(word) > 0 {
				words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
			}
		}
		return strings.Join(words, " ")
	case "snake":
		value = regexp.MustCompile(`[\s\-]+`).ReplaceAllString(value, "_")
		return strings.ToLower(value)
	case "kebab":
		value = regexp.MustCompile(`[\s_]+`).ReplaceAllString(value, "-")
		return strings.ToLower(value)
	case "slug":
		value = strings.ToLower(value)
		value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "-")
		return strings.Trim(value, "-")
	case "trim":
		return strings.TrimSpace(value)
	case "pad":
		padWidth, _ := strconv.Atoi(arg)
		if padWidth <= 0 {
			padWidth = 2
		}
		for len(value) < padWidth {
			value = "0" + value
		}
		return value
	case "trunc":
		maxLen, _ := strconv.Atoi(arg)
		if maxLen <= 0 {
			maxLen = 20
		}
		runes := []rune(value)
		if len(runes) > maxLen {
			return string(runes[:maxLen])
		}
		return value
	case "replace":
		from, to, _ := strings.Cut(arg, ":")
		return strings.ReplaceAll(value, from, to)
	case "prepend":
		return arg + value
	case "append":
		return value + arg
	case "initials":
		words := strings.Fields(value)
		initials := make([]rune, 0, len(words))
		for _, word := range words {
			runes := []rune(word)
			if len(runes) > 0 {
				initials = append(initials, runes[0])
			}
		}
		return string(initials)
	case "digits":
		var buf strings.Builder
		for _, ch := range value {
			if unicode.IsDigit(ch) {
				buf.WriteRune(ch)
			}
		}
		return buf.String()
	case "alpha":
		var buf strings.Builder
		for _, ch := range value {
			if unicode.IsLetter(ch) {
				buf.WriteRune(ch)
			}
		}
		return buf.String()
	case "sep":
		sep := "_"
		if arg != "" {
			sep = arg
		}
		return regexp.MustCompile(`[_\-\s]+`).ReplaceAllString(value, sep)
	}
	return value
}

// ── Metadata collection ───────────────────────────────────────────────────────

// Per-category variable sets used for lazy collection: media metadata is only
// fetched when at least one variable from that category appears in the template.
var imageVarNames = map[string]bool{
	"width": true, "height": true, "megapixels": true,
	"camera": true, "iso": true, "aperture": true, "shutter": true,
}
var videoVarNames = map[string]bool{
	"duration": true, "fps": true, "resolution": true, "vcodec": true,
}
var audioVarNames = map[string]bool{
	"duration": true, "artist": true, "album": true, "title": true, "track": true, "year": true,
}

// referencedTemplateVars parses the template and returns the set of variable
// names it references, so callers can skip collecting unused metadata.
func referencedTemplateVars(tmpl string) map[string]bool {
	referenced := map[string]bool{}
	for _, match := range templateTokenRe.FindAllStringSubmatch(tmpl, -1) {
		varName := strings.TrimSpace(strings.SplitN(match[1], "|", 2)[0])
		referenced[varName] = true
	}
	return referenced
}

func needsAnyOf(referenced, category map[string]bool) bool {
	for name := range category {
		if referenced[name] {
			return true
		}
	}
	return false
}

// sanitizeVarValue replaces characters that are unsafe in filenames
// (path separators, null bytes) with underscores. Applied to all values
// sourced from external metadata (EXIF, ffprobe tags) so that the output
// shown in Preview is exactly what Apply will write to disk.
func sanitizeVarValue(value string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '_'
		}
		return r
	}, value)
}

// validateNewFilename returns an error if name is not a safe, plain filename.
func validateNewFilename(name string) error {
	if name == "" {
		return fmt.Errorf("filename cannot be empty")
	}
	if strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("filename contains invalid characters")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid filename")
	}
	return nil
}

// collectFileVars builds a templateVars map for the file at absPath with the
// given 1-based index in the batch. referenced controls which media-metadata
// categories are actually fetched — if a category's variables are absent from
// the template, the expensive ffprobe / EXIF call is skipped entirely.
// All known variable names are always present in the returned map (with empty
// string for unavailable vars) so unknown-var errors only fire for typos.
func collectFileVars(absPath string, index int, referenced map[string]bool) templateVars {
	vars := templateVars{}

	// Initialise all known vars with empty strings so unknown-var errors only
	// fire for genuinely misspelled names.
	for name := range allKnownVars {
		vars[name] = ""
	}

	baseName := filepath.Base(absPath)

	// Default: derive name and ext from the path. Overridden below for directories.
	vars["ext"] = strings.TrimPrefix(filepath.Ext(absPath), ".")
	vars["name"] = strings.TrimSuffix(baseName, filepath.Ext(baseName))
	vars["index"] = strconv.Itoa(index)

	randomBytes := make([]byte, 3)
	rand.Read(randomBytes) //nolint:errcheck
	vars["random"] = hex.EncodeToString(randomBytes)

	if fi, err := os.Stat(absPath); err == nil {
		if fi.IsDir() {
			vars["name"] = baseName
			vars["ext"] = ""
		}
		vars["size"] = strconv.FormatInt(fi.Size()/1024, 10)
		vars["modified_date"] = fi.ModTime().Format("2006-01-02")
		vars["modified_time"] = fi.ModTime().Format("15-04-05")

		// Use birth time where available (Linux ext4+), fall back to mod time.
		createdAt := fi.ModTime()
		if birthTime := getBirthTime(absPath); birthTime != nil {
			createdAt = *birthTime
		}
		vars["created_date"] = createdAt.Format("2006-01-02")
		vars["created_time"] = createdAt.Format("15-04-05")
	}

	mimeType := mimeByExt("." + strings.ToLower(vars["ext"]))
	switch {
	case strings.HasPrefix(mimeType, "image/") && needsAnyOf(referenced, imageVarNames):
		collectImageVars(absPath, vars)
	case strings.HasPrefix(mimeType, "video/") && needsAnyOf(referenced, videoVarNames):
		collectVideoVars(absPath, vars)
	case strings.HasPrefix(mimeType, "audio/") && needsAnyOf(referenced, audioVarNames):
		collectAudioVars(absPath, vars)
	}

	return vars
}

func collectImageVars(absPath string, vars templateVars) {
	// Width / height via the standard image package (already used by probeImage).
	if width, height, err := probeImage(absPath); err == nil {
		vars["width"] = strconv.Itoa(width)
		vars["height"] = strconv.Itoa(height)
		mp := float64(width*height) / 1_000_000.0
		vars["megapixels"] = strconv.FormatFloat(mp, 'f', 1, 64)
	}

	// EXIF metadata (JPEG / TIFF).
	imageFile, err := os.Open(absPath)
	if err != nil {
		return
	}
	defer imageFile.Close()

	exifData, err := exif.Decode(imageFile)
	if err != nil {
		return
	}

	if tag, err := exifData.Get(exif.Model); err == nil {
		if model, err := tag.StringVal(); err == nil {
			vars["camera"] = sanitizeVarValue(strings.ReplaceAll(strings.TrimSpace(model), " ", "_"))
		}
	}

	if tag, err := exifData.Get(exif.ISOSpeedRatings); err == nil {
		if iso, err := tag.Int(0); err == nil {
			vars["iso"] = strconv.Itoa(iso)
		}
	}

	if tag, err := exifData.Get(exif.FNumber); err == nil {
		if rat, err := tag.Rat(0); err == nil {
			aperture, _ := rat.Float64()
			vars["aperture"] = fmt.Sprintf("f%.1f", aperture)
		}
	}

	if tag, err := exifData.Get(exif.ExposureTime); err == nil {
		if rat, err := tag.Rat(0); err == nil {
			numerator := rat.Num().Int64()
			denominator := rat.Denom().Int64()
			if denominator == 0 || denominator == 1 {
				vars["shutter"] = fmt.Sprintf("%ds", numerator)
			} else if numerator == 1 {
				vars["shutter"] = fmt.Sprintf("1_%ds", denominator)
			} else {
				vars["shutter"] = fmt.Sprintf("%d_%ds", numerator, denominator)
			}
		}
	}

	// Override created_date/time with EXIF DateTime if available.
	if exifTime, err := exifData.DateTime(); err == nil {
		vars["created_date"] = exifTime.Format("2006-01-02")
		vars["created_time"] = exifTime.Format("15-04-05")
	}
}

// ffprobeResult is a minimal subset of ffprobe JSON output used for media vars.
type ffprobeResult struct {
	Streams []struct {
		CodecType   string `json:"codec_type"`
		CodecName   string `json:"codec_name"`
		Width       int    `json:"width"`
		Height      int    `json:"height"`
		RFrameRate  string `json:"r_frame_rate"` // e.g. "30/1" or "24000/1001"
		Duration    string `json:"duration"`
	} `json:"streams"`
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

func runFFprobe(absPath string) (*ffprobeResult, error) {
	out, err := exec.Command(
		"ffprobe", "-v", "quiet",
		"-print_format", "json",
		"-show_streams", "-show_format",
		absPath,
	).Output()
	if err != nil {
		return nil, err
	}
	var result ffprobeResult
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func collectVideoVars(absPath string, vars templateVars) {
	probe, err := runFFprobe(absPath)
	if err != nil {
		return
	}

	for _, stream := range probe.Streams {
		if stream.CodecType != "video" {
			continue
		}

		vars["vcodec"] = strings.ToUpper(stream.CodecName)
		vars["resolution"] = resolutionShorthand(stream.Height)

		// Frame rate: stored as "num/den" string (e.g. "30/1", "24000/1001").
		if stream.RFrameRate != "" {
			parts := strings.SplitN(stream.RFrameRate, "/", 2)
			if len(parts) == 2 {
				fpsNum, _ := strconv.ParseFloat(parts[0], 64)
				fpsDen, _ := strconv.ParseFloat(parts[1], 64)
				if fpsDen > 0 {
					vars["fps"] = strconv.FormatFloat(math.Round(fpsNum/fpsDen), 'f', 0, 64)
				}
			}
		}

		// Duration: prefer stream-level, fall back to format-level.
		durationStr := stream.Duration
		if durationStr == "" {
			durationStr = probe.Format.Duration
		}
		if durationStr != "" {
			if seconds, err := strconv.ParseFloat(durationStr, 64); err == nil {
				vars["duration"] = formatMediaDuration(seconds)
			}
		}
		break
	}
}

func collectAudioVars(absPath string, vars templateVars) {
	probe, err := runFFprobe(absPath)
	if err != nil {
		return
	}

	if probe.Format.Duration != "" {
		if seconds, err := strconv.ParseFloat(probe.Format.Duration, 64); err == nil {
			vars["duration"] = formatMediaDuration(seconds)
		}
	}

	// ID3 / Vorbis / MP4 tags are normalised to lowercase keys by ffprobe.
	getTag := func(keys ...string) string {
		for _, key := range keys {
			if value := probe.Format.Tags[key]; value != "" {
				return value
			}
			if value := probe.Format.Tags[strings.ToLower(key)]; value != "" {
				return value
			}
		}
		return ""
	}

	if value := getTag("artist", "ARTIST", "TPE1"); value != "" {
		vars["artist"] = sanitizeVarValue(value)
	}
	if value := getTag("album", "ALBUM", "TALB"); value != "" {
		vars["album"] = sanitizeVarValue(value)
	}
	if value := getTag("title", "TITLE", "TIT2"); value != "" {
		vars["title"] = sanitizeVarValue(value)
	}
	if value := getTag("track", "TRACK", "TRCK"); value != "" {
		// Strip track total (e.g. "1/12" → "1").
		vars["track"] = sanitizeVarValue(strings.SplitN(value, "/", 2)[0])
	}
	if value := getTag("date", "DATE", "year", "YEAR", "TDRC"); value != "" {
		if len(value) >= 4 {
			vars["year"] = sanitizeVarValue(value[:4])
		}
	}
}

func resolutionShorthand(height int) string {
	switch {
	case height >= 2160:
		return "4K"
	case height >= 1440:
		return "1440p"
	case height >= 1080:
		return "1080p"
	case height >= 720:
		return "720p"
	case height >= 480:
		return "480p"
	default:
		return strconv.Itoa(height) + "p"
	}
}

func formatMediaDuration(seconds float64) string {
	total := int(math.Round(seconds))
	hours := total / 3600
	minutes := (total % 3600) / 60
	secs := total % 60
	return fmt.Sprintf("%02d-%02d-%02d", hours, minutes, secs)
}

// ── HTTP endpoints ────────────────────────────────────────────────────────────

// POST /api/v1/rename/preview
// Evaluates the template against each provided file and returns the list of
// computed new names without performing any actual renames.
func (h *AdvancedRenamerHandler) Preview(c *gin.Context) {
	root := userRoot(c)

	var body renamePreviewRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(body.Paths) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "paths required"})
		return
	}

	referenced := referencedTemplateVars(body.Template)

	items := make([]renamePreviewItem, 0, len(body.Paths))
	for i, relPath := range body.Paths {
		absPath, err := safePath(root, relPath)
		if err != nil {
			items = append(items, renamePreviewItem{
				OriginalName: filepath.Base(relPath),
				Error:        err.Error(),
			})
			continue
		}

		vars := collectFileVars(absPath, i+1, referenced)
		newName, unknowns := renderTemplate(body.Template, vars)

		item := renamePreviewItem{
			OriginalName: filepath.Base(absPath),
			NewName:      newName,
		}
		if len(unknowns) > 0 {
			item.Error = "unknown: " + strings.Join(unknowns, ", ")
		} else if err := validateNewFilename(newName); err != nil {
			item.Error = err.Error()
		}
		items = append(items, item)
	}

	c.JSON(http.StatusOK, gin.H{"items": items})
}

// POST /api/v1/rename/apply
// Executes a batch rename. Detects conflicts and returns HTTP 409 when
// conflict_mode is "error" (the default). On success, saves the template as
// the last-used template for the folder.
func (h *AdvancedRenamerHandler) Apply(c *gin.Context) {
	root := userRoot(c)
	userID := c.GetInt64("userID")

	var body renameApplyRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(body.Renames) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "renames required"})
		return
	}
	if body.ConflictMode == "" {
		body.ConflictMode = "error"
	}

	// Resolve absolute paths once.
	type resolvedOp struct {
		srcAbs string
		dstDir string
		toName string
	}
	ops := make([]resolvedOp, 0, len(body.Renames))
	for _, op := range body.Renames {
		if err := validateNewFilename(op.To); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid destination filename: " + err.Error()})
			return
		}
		srcAbs, err := safePath(root, op.From)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ops = append(ops, resolvedOp{
			srcAbs: srcAbs,
			dstDir: filepath.Dir(srcAbs),
			toName: op.To,
		})
	}

	// Check / resolve conflicts.
	switch body.ConflictMode {
	case "error":
		var conflicts []string
		for _, op := range ops {
			dstAbs := filepath.Join(op.dstDir, op.toName)
			if dstAbs == op.srcAbs {
				continue // no-op rename
			}
			if _, err := os.Stat(dstAbs); err == nil {
				conflicts = append(conflicts, op.toName)
			}
		}
		if len(conflicts) > 0 {
			c.JSON(http.StatusConflict, gin.H{"error": "conflict", "conflicts": conflicts})
			return
		}

	case "rename":
		// Build per-directory maps of existing names (minus the files being
		// renamed, since they vacate their current slots) and track claimed names.
		existingByDir := map[string]map[string]bool{}
		for _, op := range ops {
			if _, ok := existingByDir[op.dstDir]; ok {
				continue
			}
			existingByDir[op.dstDir] = map[string]bool{}
			if entries, err := os.ReadDir(op.dstDir); err == nil {
				for _, entry := range entries {
					existingByDir[op.dstDir][entry.Name()] = true
				}
			}
		}
		for _, op := range ops {
			delete(existingByDir[op.dstDir], filepath.Base(op.srcAbs))
		}
		claimedByDir := map[string]map[string]bool{}
		for i, op := range ops {
			if _, ok := claimedByDir[op.dstDir]; !ok {
				claimedByDir[op.dstDir] = map[string]bool{}
			}
			ops[i].toName = findFreeName(op.toName, existingByDir[op.dstDir], claimedByDir[op.dstDir])
			claimedByDir[op.dstDir][ops[i].toName] = true
		}
	}

	// Execute renames.
	for _, op := range ops {
		dstAbs := filepath.Join(op.dstDir, op.toName)
		if dstAbs == op.srcAbs {
			continue // nothing to do
		}

		if body.ConflictMode == "replace" {
			if _, err := os.Stat(dstAbs); err == nil {
				if err := os.RemoveAll(dstAbs); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "could not overwrite: " + err.Error()})
					return
				}
			}
		}

		if err := os.Rename(op.srcAbs, dstAbs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	// Persist the template as the last-used template for this folder.
	if body.Template != "" {
		h.db.Exec( //nolint:errcheck
			`INSERT INTO folder_prefs (user_id, path, view_mode, last_rename_template)
			 VALUES (?, ?, 'list', ?)
			 ON CONFLICT(user_id, path) DO UPDATE SET last_rename_template = excluded.last_rename_template`,
			userID, body.Folder, body.Template,
		)
	}

	c.JSON(http.StatusOK, gin.H{"message": "renamed"})
}

// GET /api/v1/rename/presets
// Returns all saved rename presets for the authenticated user.
func (h *AdvancedRenamerHandler) ListPresets(c *gin.Context) {
	userID := c.GetInt64("userID")

	rows, err := h.db.Query(
		`SELECT id, name, template, created_at FROM rename_presets WHERE user_id = ? ORDER BY name`,
		userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	presets := make([]renamePresetRow, 0)
	for rows.Next() {
		var preset renamePresetRow
		if err := rows.Scan(&preset.ID, &preset.Name, &preset.Template, &preset.CreatedAt); err != nil {
			continue
		}
		presets = append(presets, preset)
	}
	c.JSON(http.StatusOK, presets)
}

// POST /api/v1/rename/presets
// Creates or replaces a named preset for the authenticated user.
func (h *AdvancedRenamerHandler) SavePreset(c *gin.Context) {
	userID := c.GetInt64("userID")

	var body struct {
		Name     string `json:"name"`
		Template string `json:"template"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Name == "" || body.Template == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and template required"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var preset renamePresetRow
	err := h.db.QueryRow(
		`INSERT INTO rename_presets (user_id, name, template, created_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(user_id, name) DO UPDATE SET template = excluded.template
		 RETURNING id, name, template, created_at`,
		userID, body.Name, body.Template, now,
	).Scan(&preset.ID, &preset.Name, &preset.Template, &preset.CreatedAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, preset)
}

// DELETE /api/v1/rename/presets/:id
// Deletes a preset owned by the authenticated user.
func (h *AdvancedRenamerHandler) DeletePreset(c *gin.Context) {
	userID := c.GetInt64("userID")
	presetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	result, err := h.db.Exec(
		`DELETE FROM rename_presets WHERE id = ? AND user_id = ?`,
		presetID, userID,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "preset not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// GET /api/v1/rename/folder-template?folder=
// Returns the last-used rename template for the given folder path.
func (h *AdvancedRenamerHandler) GetFolderTemplate(c *gin.Context) {
	userID := c.GetInt64("userID")
	folder := c.Query("folder")

	var template string
	err := h.db.QueryRow(
		`SELECT last_rename_template FROM folder_prefs WHERE user_id = ? AND path = ?`,
		userID, folder,
	).Scan(&template)
	if err != nil {
		// No stored pref — return empty (frontend uses its default).
		c.JSON(http.StatusOK, gin.H{"template": ""})
		return
	}
	c.JSON(http.StatusOK, gin.H{"template": template})
}
