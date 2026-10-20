package handlers

import (
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestSubtitleTag(t *testing.T) {
	tests := []struct {
		fileName string
		tag      string
		ok       bool
	}{
		{"movie.srt", "", true},
		{"movie.en.srt", "en", true},
		{"movie.en.forced.srt", "en.forced", true},
		{"Movie.EN.SRT", "EN", true},
		{"MOVIE.vtt", "", true},
		{"movie..ass", "", true},
		{"movie.mkv", "", false},       // not a subtitle
		{"movie.en.txt", "", false},    // not a subtitle
		{"movies.srt", "", false},      // another video's name
		{"movie-extra.srt", "", false}, // no dot after the name
		{"mov.srt", "", false},
		{".srt", "", false},
	}
	for _, test := range tests {
		tag, ok := subtitleTag("movie", test.fileName)
		if tag != test.tag || ok != test.ok {
			t.Errorf("subtitleTag(movie, %q) = %q, %v; want %q, %v", test.fileName, tag, ok, test.tag, test.ok)
		}
	}
}

func TestSubtitleList(t *testing.T) {
	// movie.srt used to crash the listing, so the video got no subtitles at all.
	annRoot, _ := newUserFolders(t)
	writeFiles(t, annRoot, map[string]string{
		"videos/movie.mkv":    "not really a video",
		"videos/movie.srt":    "1",
		"videos/movie.en.srt": "1",
		"videos/movies.srt":   "1",
	})
	request := httptest.NewRequest(http.MethodGet, "/?path="+url.QueryEscape("videos/movie.mkv"), nil)
	response := serve(NewSubtitlesHandler().List, annRoot, request)
	assertStatus(t, response, http.StatusOK)

	tracks := decodeResponse[[]SubtitleTrack](t, response)
	got := map[string]string{}
	for _, track := range tracks {
		got[track.URL] = track.Label
	}
	want := map[string]string{
		"/api/v1/files/subtitle?path=" + url.QueryEscape("videos/movie.srt"):    "Default",
		"/api/v1/files/subtitle?path=" + url.QueryEscape("videos/movie.en.srt"): "English",
	}
	if !maps.Equal(got, want) {
		t.Errorf("tracks = %v, want %v", got, want)
	}
}
