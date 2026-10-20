// Package media runs ffmpeg, ffprobe and ImageMagick on user files safely.
//
// These tools decide a file's format from its contents, not its name, and
// some formats they support aren't media: ffmpeg's concat/playlist demuxers
// read other files listed inside, and its protocols can reach the network and
// internal services; ImageMagick can likewise be steered into reading other
// files or URLs. A crafted file with an ordinary name could otherwise make a
// thumbnail or preview read another user's files, the database, or a service
// on the network. So every ffmpeg/ffprobe input is restricted to the local
// file and real media formats, and ImageMagick runs under a policy that
// disables the formats that reference other files.
package media

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// allowedFormats are ffmpeg/ffprobe demuxers for the audio, video and
// subtitle formats the app handles. Playlist/aggregating demuxers (concat,
// hls, …) are deliberately absent, so a file can't list other files to read.
const allowedFormats = "mov,mp4,m4a,3gp,3g2,mj2,matroska,webm,avi,mpegts,mpeg," +
	"mpegvideo,asf,flv,ogg,mp3,mp2,flac,wav,aiff,aac,ac3,eac3,dts,amr,ape,wv," +
	"tak,tta,dsf,mpc,caf,dv,w64,au,gif,webvtt,srt,ass,jacosub,microdvd,subviewer"

// inputGuard keeps an ffmpeg/ffprobe input to the local file (no network or
// other protocols) and to a real media format (no concat/playlist demuxers).
// It goes before the -i it guards, among the other input options.
func inputGuard() []string {
	return []string{"-protocol_whitelist", "file", "-format_whitelist", allowedFormats}
}

// FFprobe builds an ffprobe command for the real local file at path, with the
// given options (everything except the input), guarded.
func FFprobe(path string, options ...string) *exec.Cmd {
	args := append(options, inputGuard()...)
	args = append(args, "-i", path)
	return exec.Command("ffprobe", args...)
}

// FFprobeContext is FFprobe bound to ctx.
func FFprobeContext(ctx context.Context, path string, options ...string) *exec.Cmd {
	args := append(options, inputGuard()...)
	args = append(args, "-i", path)
	return Command(ctx, "ffprobe", args...)
}

// FFmpeg builds an ffmpeg command reading the real local file input, guarded
// like FFprobe: inputOptions go before the input (e.g. -ss), outputArgs after.
// It runs at low priority (nice), using only spare CPU.
func FFmpeg(ctx context.Context, input string, inputOptions []string, outputArgs ...string) *exec.Cmd {
	args := append([]string{"-n", "10", "ffmpeg"}, inputOptions...)
	args = append(args, inputGuard()...)
	args = append(args, "-i", input)
	args = append(args, outputArgs...)
	return Command(ctx, "nice", args...)
}

// MagickConvert builds an ImageMagick command (the `convert`/`magick` tool)
// with the given arguments, under a policy that disables formats able to read
// other files or URLs (we don't need them: SVG goes through rsvg-convert and
// PDF through pdftoppm). The policy is passed in the environment, so it also
// covers the delegates ImageMagick may call.
func MagickConvert(ctx context.Context, args ...string) *exec.Cmd {
	return magick(ctx, "convert", args...)
}

// MagickIdentify builds ImageMagick's `identify` command, under the same policy.
func MagickIdentify(ctx context.Context, args ...string) *exec.Cmd {
	return magick(ctx, "identify", args...)
}

func magick(ctx context.Context, tool string, args ...string) *exec.Cmd {
	cmd := Command(ctx, tool, args...)
	cmd.Env = append(cmd.Environ(), "MAGICK_CONFIGURE_PATH="+magickPolicyDir())
	return cmd
}

// Command is exec.CommandContext that also stops waiting for the tool's
// output shortly after ctx ends, so a hung tool can't hold its caller.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// disabledMagickCoders are ImageMagick formats that can reference other files
// or fetch URLs, none of which the app needs.
var disabledMagickCoders = []string{
	"HTTPS", "HTTP", "FTP", "MSL", "TEXT", "LABEL", "CAPTION", "URL",
	"EPHEMERAL", "EPT", "PANGO", "SVG", "MSVG", "PS", "PS2", "PS3",
	"EPS", "PDF", "XPS", "MVG", "SHOW", "WIN", "X",
}

// magickPolicyDir returns a directory holding a policy.xml that disables the
// coders above, written once. On failure it returns "", so ImageMagick runs
// under its own default policy rather than failing outright.
var magickPolicyDir = sync.OnceValue(func() string {
	dir, err := os.MkdirTemp("", "magick-policy-")
	if err != nil {
		return ""
	}
	if err := os.WriteFile(filepath.Join(dir, "policy.xml"), []byte(magickPolicy()), 0o644); err != nil {
		return ""
	}
	return dir
})

// magickPolicy is the ImageMagick policy.xml that disables those coders and
// the indirect-read feature ("@file" argument expansion).
func magickPolicy() string {
	var policies strings.Builder
	policies.WriteString(`<policymap>` + "\n")
	policies.WriteString(`  <policy domain="path" rights="none" pattern="@*"/>` + "\n")
	for _, coder := range disabledMagickCoders {
		policies.WriteString(`  <policy domain="coder" rights="none" pattern="` + coder + `"/>` + "\n")
	}
	policies.WriteString(`</policymap>` + "\n")
	return policies.String()
}
