package bridge

import (
	"testing"
	"time"

	"github.com/jacobgad/obs-controller/internal/obs"
)

func TestSceneNamesReversesToUIOrder(t *testing.T) {
	scenes := []obs.Scene{
		{SceneName: "Bottom"},
		{SceneName: "Middle"},
		{SceneName: "Top"},
	}
	got := sceneNames(scenes)
	want := []string{"Top", "Middle", "Bottom"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sceneNames = %v, want %v", got, want)
		}
	}
}

func TestPickImageFormat(t *testing.T) {
	cases := []struct {
		supported []string
		want      string
	}{
		{[]string{"bmp", "jpeg", "jpg", "png"}, "jpg"},
		{[]string{"jpeg", "png"}, "jpeg"},
		{[]string{"png"}, "png"},
		{nil, "png"},
	}
	for _, tc := range cases {
		if got := pickImageFormat(tc.supported); got != tc.want {
			t.Errorf("pickImageFormat(%v) = %q, want %q", tc.supported, got, tc.want)
		}
	}
}

func TestWSMajorVersion(t *testing.T) {
	cases := map[string]int{"5.5.2": 5, "4.9.1": 4, "": 0, "abc": 0}
	for in, want := range cases {
		if got := wsMajorVersion(in); got != want {
			t.Errorf("wsMajorVersion(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestJitterStaysBounded(t *testing.T) {
	base := 10 * time.Second
	for range 100 {
		d := jitter(base)
		if d < 8*time.Second || d > 12*time.Second {
			t.Fatalf("jitter(%v) = %v outside ±20%%", base, d)
		}
	}
}

func TestParseOnOff(t *testing.T) {
	if v, err := parseOnOff("ON"); err != nil || !v {
		t.Errorf("ON = %v, %v", v, err)
	}
	if v, err := parseOnOff("OFF"); err != nil || v {
		t.Errorf("OFF = %v, %v", v, err)
	}
	if _, err := parseOnOff("on"); err == nil {
		t.Error("lowercase accepted")
	}
}
