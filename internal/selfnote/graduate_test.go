package selfnote_test

import (
	"strings"
	"testing"

	"github.com/shotah/ai-gantry/internal/selfnote"
)

func TestGraduateVoice_AppendsNewJokeOnce(t *testing.T) {
	s, err := selfnote.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prior := "Facts: leftover\nVoice: dry"
	next := "Facts: leftover\nVoice: dry; gag: \"that gull had a mortgage\""
	ok, err := selfnote.GraduateVoice(s, prior, next)
	if err != nil || !ok {
		t.Fatalf("first = %v, %v", ok, err)
	}
	got, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "gull") {
		t.Fatalf("SELF.md missing gag: %q", got)
	}
	ok, err = selfnote.GraduateVoice(s, next, next)
	if err != nil || ok {
		t.Fatalf("unchanged = %v, %v", ok, err)
	}
	// Same joke already on disk — even if Voice restates it.
	restated := "Facts: leftover\nVoice: gag: \"that gull had a mortgage\""
	ok, err = selfnote.GraduateVoice(s, prior, restated)
	if err != nil || ok {
		t.Fatalf("already in SELF = %v, %v", ok, err)
	}
	again, _ := s.Read()
	if strings.Count(again, "gull") != strings.Count(got, "gull") {
		t.Fatalf("duplicated gag:\n%s", again)
	}
}

func TestGraduateVoice_SkipsMoodWeather(t *testing.T) {
	s, err := selfnote.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ok, err := selfnote.GraduateVoice(s,
		"Facts: x\nVoice: dry",
		"Facts: x\nVoice: dry today",
	)
	if err != nil || ok {
		t.Fatalf("mood = %v, %v", ok, err)
	}
}
