package bridge

import (
	"testing"
	"time"

	"github.com/andreykaipov/goobs/api/typedefs"
)

func TestSceneNamesReversesToUIOrder(t *testing.T) {
	scenes := []*typedefs.Scene{
		{SceneName: "Bottom", SceneIndex: 0},
		{SceneName: "Middle", SceneIndex: 1},
		{SceneName: "Top", SceneIndex: 2},
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

func TestPercent(t *testing.T) {
	if got := percent(5, 200); got != 2.5 {
		t.Errorf("percent(5, 200) = %v", got)
	}
	if got := percent(5, 0); got != 0 {
		t.Errorf("percent(5, 0) = %v", got)
	}
}

func TestInterval(t *testing.T) {
	if interval(true, time.Second, time.Minute) != time.Second {
		t.Error("active interval not used")
	}
	if interval(false, time.Second, time.Minute) != time.Minute {
		t.Error("idle interval not used")
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
