// Файл: internal/window/browser.go
// Package window — окно программы (Chromium --app) и открытие файлов/папок в ОС. Перенос из doc-agent.
package window

import (
	"errors"
	"path/filepath"
	"time"
)

var (
	ErrNoDisplay    = errors.New("нет графической среды (DISPLAY и WAYLAND_DISPLAY не заданы)")
	ErrNoAppBrowser = errors.New("не найден Chrome, Edge или Chromium для окна программы")
)

// MinWindowLife — окно, закрывшееся быстрее, считается не открывшимся (браузер не запустился).
const MinWindowLife = 3 * time.Second

func LivedLongEnough(d time.Duration) bool { return d >= MinWindowLife }

// AppBrowser — путь к браузеру на Chromium, умеющему окно приложения (--app); "" — такого нет.
func AppBrowser(goos string, lookPath func(string) (string, error), exists func(string) bool, getenv func(string) string) string {
	var candidates []string
	switch goos {
	case "windows":
		for _, base := range []string{getenv("ProgramFiles(x86)"), getenv("ProgramFiles"), getenv("LocalAppData")} {
			if base == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
		}
	case "darwin":
		candidates = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
		}
	default:
		for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "microsoft-edge-stable", "brave-browser"} {
			if p, err := lookPath(name); err == nil {
				return p
			}
		}
		return ""
	}
	for _, p := range candidates {
		if exists(p) {
			return p
		}
	}
	return ""
}

func WindowArgs(link, profile string) []string {
	return []string{
		"--app=" + link,
		"--user-data-dir=" + profile,
		"--class=pylesos", // по нему система связывает окно с ярлыком
		"--no-first-run",
		"--no-default-browser-check",
		"--window-size=1200,900",
	}
}
