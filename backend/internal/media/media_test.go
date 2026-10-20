package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInputGuardsEveryCall(t *testing.T) {
	cmd := FFprobe("/videos/clip.mp4", "-v", "error")
	want := []string{"ffprobe", "-v", "error", "-protocol_whitelist", "file", "-format_whitelist", allowedFormats, "-i", "/videos/clip.mp4"}
	if !slices.Equal(cmd.Args, want) {
		t.Errorf("FFprobe args = %v", cmd.Args)
	}

	ffmpeg := FFmpeg(context.Background(), "/videos/clip.mp4", []string{"-ss", "5"}, "-frames:v", "1", "out.png")
	wantFFmpeg := []string{"nice", "-n", "10", "ffmpeg", "-ss", "5", "-protocol_whitelist", "file", "-format_whitelist", allowedFormats, "-i", "/videos/clip.mp4", "-frames:v", "1", "out.png"}
	if !slices.Equal(ffmpeg.Args, wantFFmpeg) {
		t.Errorf("FFmpeg args = %v", ffmpeg.Args)
	}
}

func TestMagickConvertSetsPolicy(t *testing.T) {
	cmd := MagickConvert(context.Background(), "in.png", "out.jpg")
	var policyPath string
	for _, env := range cmd.Env {
		if strings.HasPrefix(env, "MAGICK_CONFIGURE_PATH=") {
			policyPath = strings.TrimPrefix(env, "MAGICK_CONFIGURE_PATH=")
		}
	}
	if policyPath == "" {
		t.Fatal("MAGICK_CONFIGURE_PATH not set")
	}
	data, err := os.ReadFile(filepath.Join(policyPath, "policy.xml"))
	if err != nil {
		t.Fatalf("policy.xml: %v", err)
	}
	for _, want := range []string{`pattern="@*"`, `pattern="HTTPS"`, `pattern="MSL"`, `pattern="PDF"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("policy.xml missing %s", want)
		}
	}
}

// ── Integration (need the tools; the dev container has them) ────────────────

func hasTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func TestFFprobeRefusesConcatDisguisedAsVideo(t *testing.T) {
	if !hasTool("ffprobe") {
		t.Skip("needs ffprobe")
	}
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret.txt")
	os.WriteFile(secret, []byte("another user's file"), 0o644)
	// A concat playlist named like a video, pointing at the secret.
	attack := filepath.Join(dir, "clip.mp4")
	os.WriteFile(attack, []byte("ffconcat version 1.0\nfile '"+secret+"'\n"), 0o644)

	out, err := FFprobe(attack, "-v", "error", "-show_entries", "format=format_name", "-of", "csv=p=0").CombinedOutput()
	if err == nil {
		t.Errorf("concat file was accepted: %s", out)
	}
	if strings.Contains(string(out), "another user") {
		t.Errorf("the secret leaked: %s", out)
	}
}

func TestFFprobeAcceptsRealMedia(t *testing.T) {
	if !hasTool("ffmpeg") || !hasTool("ffprobe") {
		t.Skip("needs ffmpeg and ffprobe")
	}
	dir := t.TempDir()
	for _, name := range []string{"a.mp4", "a.mkv", "a.mp3", "a.wav"} {
		path := filepath.Join(dir, name)
		if out, err := exec.Command("ffmpeg", "-loglevel", "error", "-f", "lavfi", "-i", "sine=duration=1", "-y", path).CombinedOutput(); err != nil {
			t.Fatalf("make %s: %v %s", name, err, out)
		}
		out, err := FFprobe(path, "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0").Output()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			t.Errorf("%s not probed: %v (%s)", name, err, out)
		}
	}
}

func TestMagickConvertBlocksRemoteAndFileReads(t *testing.T) {
	if !hasTool("convert") {
		t.Skip("needs ImageMagick")
	}
	dir := t.TempDir()
	out, err := MagickConvert(context.Background(), "https://example.com/x.png", filepath.Join(dir, "out.png")).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "security policy") {
		t.Errorf("https coder not blocked: %v %s", err, out)
	}

	list := filepath.Join(dir, "list.txt")
	os.WriteFile(list, []byte("/etc/hostname\n"), 0o644)
	out, err = MagickConvert(context.Background(), "@"+list, filepath.Join(dir, "out2.png")).CombinedOutput()
	if err == nil || !strings.Contains(string(out), "security policy") {
		t.Errorf("@file read not blocked: %v %s", err, out)
	}
}

func TestMagickConvertStillConverts(t *testing.T) {
	if !hasTool("convert") {
		t.Skip("needs ImageMagick")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "in.png")
	if out, err := exec.Command("magick", "-size", "40x40", "xc:red", src).CombinedOutput(); err != nil {
		t.Skipf("cannot make a test image: %s", out)
	}
	dst := filepath.Join(dir, "out.jpg")
	if out, err := MagickConvert(context.Background(), src, "-resize", "20x20", dst).CombinedOutput(); err != nil {
		t.Fatalf("convert under policy failed: %v %s", err, out)
	}
	if info, err := os.Stat(dst); err != nil || info.Size() == 0 {
		t.Errorf("no output produced: %v", err)
	}
}
