// Файл: internal/screens/settings.go
package screens

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"pylesos/internal/ollama"
	settingsdomain "pylesos/internal/settings/domain"
	"pylesos/internal/texts"
)

func (b *Builder) Settings(ctx context.Context) (SettingsScreen, error) {
	s, err := b.d.Settings.Get(ctx)
	if err != nil {
		return SettingsScreen{}, fmt.Errorf("экран «Настройки»: %w", err)
	}
	screen := settingsScreen(valuesOf(s), nil, "")
	screen.OllamaLabel, screen.OllamaStatus = texts.SettingsOllama, b.ollamaStatus(ctx)
	return screen, nil
}

// ollamaStatus — работает ли Ollama и есть ли модель (не дольше 3 с: экран не должен ждать).
func (b *Builder) ollamaStatus(ctx context.Context) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	models, err := b.d.Ollama.VisionModels(ctx)
	if err != nil {
		return texts.OllamaDown
	}
	if model := ollama.Preferred(models); model != "" {
		return fmt.Sprintf(texts.OllamaOK, model)
	}
	return texts.OllamaNoModel
}

// SaveSettings сохраняет ввод; при ошибках возвращает экран с введённым текстом и ошибками у полей.
func (b *Builder) SaveSettings(ctx context.Context, input map[string]string) (SettingsScreen, error) {
	saved, problems, err := b.d.Settings.Save(ctx, input)
	if err != nil {
		return SettingsScreen{}, fmt.Errorf("сохранить настройки: %w", err)
	}
	if len(problems) > 0 {
		screen := settingsScreen(input, problems, "")
		screen.OllamaLabel, screen.OllamaStatus = texts.SettingsOllama, b.ollamaStatus(ctx)
		return screen, nil
	}
	screen := settingsScreen(valuesOf(saved), nil, texts.SettingsSaved)
	screen.OllamaLabel, screen.OllamaStatus = texts.SettingsOllama, b.ollamaStatus(ctx)
	return screen, nil
}

func valuesOf(s settingsdomain.Settings) map[string]string {
	return map[string]string{
		settingsdomain.KeyExportsDir:    s.ExportsDir,
		settingsdomain.KeyPhonePauseSec: strconv.Itoa(s.PhonePauseSec),
	}
}

func settingsScreen(values, problems map[string]string, message string) SettingsScreen {
	field := func(key, label, kind string) Field {
		return Field{Key: key, Label: label, Value: values[key], Kind: kind, Error: problems[key]}
	}
	return SettingsScreen{
		Title: texts.SettingsTitle,
		Fields: []Field{
			field(settingsdomain.KeyExportsDir, texts.SettingsExportsDir, "text"),
			field(settingsdomain.KeyPhonePauseSec, texts.SettingsPhonePause, "number"),
		},
		SaveLabel:   texts.SettingsSave,
		SavingLabel: texts.Saving,
		Message:     message,
	}
}
