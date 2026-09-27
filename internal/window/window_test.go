// Файл: internal/window/window_test.go
package window

import (
	"errors"
	"slices"
	"testing"
	"time"
)

func TestAppBrowser(t *testing.T) {
	look := func(found string) func(string) (string, error) {
		return func(name string) (string, error) {
			if name == found {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("нет")
		}
	}
	none := func(string) bool { return false }
	env := func(k string) string {
		if k == "ProgramFiles" {
			return `C:\PF`
		}
		return ""
	}
	if got := AppBrowser("linux", look("chromium"), none, env); got != "/usr/bin/chromium" {
		t.Fatalf("linux: %q", got)
	}
	if got := AppBrowser("linux", look("нет-такого"), none, env); got != "" {
		t.Fatalf("linux без браузера: %q", got)
	}
	edge := func(p string) bool {
		return p == `C:\PF/Microsoft/Edge/Application/msedge.exe` || p == `C:\PF\Microsoft\Edge\Application\msedge.exe`
	}
	if got := AppBrowser("windows", look(""), edge, env); got == "" {
		t.Fatal("windows: Edge не найден")
	}
}

func TestWindowArgs(t *testing.T) {
	args := WindowArgs("http://127.0.0.1:1/?t=x", "/data/window")
	for _, want := range []string{"--app=http://127.0.0.1:1/?t=x", "--user-data-dir=/data/window", "--class=pylesos"} {
		if !slices.Contains(args, want) {
			t.Fatalf("нет %q в %v", want, args)
		}
	}
}

func TestOpenCommand(t *testing.T) {
	noDisplay := func(string) string { return "" }
	if name, _, err := OpenCommand("windows", `C:\x`, true, noDisplay); err != nil || name != "explorer.exe" {
		t.Fatalf("windows папка: %q %v", name, err)
	}
	if _, _, err := OpenCommand("linux", "/x", true, noDisplay); !errors.Is(err, ErrNoDisplay) {
		t.Fatalf("linux без экрана: %v", err)
	}
}

func TestOpenAppWithoutDisplayOrBrowser(t *testing.T) {
	o := Opener{GOOS: "linux", Getenv: func(string) string { return "" }}
	if _, err := o.OpenApp("http://x"); !errors.Is(err, ErrNoDisplay) {
		t.Fatalf("без экрана: %v", err)
	}
	o = Opener{GOOS: "linux", Getenv: func(k string) string { return map[string]string{"DISPLAY": ":0"}[k] },
		LookPath: func(string) (string, error) { return "", errors.New("нет") }, Exists: func(string) bool { return false }}
	if _, err := o.OpenApp("http://x"); !errors.Is(err, ErrNoAppBrowser) {
		t.Fatalf("без браузера: %v", err)
	}
}

func TestLivedLongEnough(t *testing.T) {
	if LivedLongEnough(time.Second) || !LivedLongEnough(5*time.Second) {
		t.Fatal("граница 3 секунды")
	}
}
