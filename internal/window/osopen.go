// Файл: internal/window/osopen.go
package window

// OpenCommand выбирает программу для открытия target. Папки на Windows — через explorer.exe
// (rundll32 url.dll ломает не-ASCII пути). На Linux без экрана (SSH) — сразу ошибка.
func OpenCommand(goos, target string, dir bool, getenv func(string) string) (string, []string, error) {
	switch goos {
	case "windows":
		if dir {
			return "explorer.exe", []string{target}, nil
		}
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}, nil
	case "darwin":
		return "open", []string{target}, nil
	default:
		if getenv("DISPLAY") == "" && getenv("WAYLAND_DISPLAY") == "" {
			return "", nil, ErrNoDisplay
		}
		return "xdg-open", []string{target}, nil
	}
}

// OpenInOS открывает адрес в браузере по умолчанию, файл — в его программе, папку — в файловом менеджере.
func (o Opener) OpenInOS(target string, dir bool) error {
	name, args, err := OpenCommand(o.GOOS, target, dir, o.Getenv)
	if err != nil {
		return err
	}
	wait, err := o.Start(name, args...)
	if err != nil {
		return err
	}
	go func() {
		if err := wait(); err != nil {
			o.Log.Warn("открыть в ОС", "target", target, "err", err)
		}
	}()
	return nil
}
