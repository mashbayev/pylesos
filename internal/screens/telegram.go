// Файл: internal/screens/telegram.go
package screens

import (
	"context"
	"errors"
	"fmt"

	tgservice "pylesos/internal/telegram/service"
	"pylesos/internal/texts"
)

func (b *Builder) Telegram(ctx context.Context) (TelegramScreen, error) {
	return b.telegram(ctx, nil, nil, "", false)
}

// TelegramSubmit — шаг входа; при ошибках форма возвращается с введённым (кроме пароля) и ошибками у полей.
func (b *Builder) TelegramSubmit(ctx context.Context, values map[string]string) (TelegramScreen, error) {
	problems, err := b.d.Login.Submit(ctx, values)
	if err != nil {
		return TelegramScreen{}, fmt.Errorf("вход в Telegram: %w", err)
	}
	if len(problems) > 0 {
		return b.telegram(ctx, values, problems, "", false)
	}
	return b.telegram(ctx, nil, nil, "", false)
}

func (b *Builder) TelegramAddGroup(ctx context.Context, key string) (TelegramScreen, error) {
	err := b.d.Groups.Add(ctx, key)
	if errors.Is(err, tgservice.ErrGroupGone) {
		return b.telegram(ctx, nil, nil, texts.ErrTgGroupGone, false)
	}
	if err != nil {
		return TelegramScreen{}, fmt.Errorf("добавить группу: %w", err)
	}
	return b.telegram(ctx, nil, nil, texts.TgGroupAdded, false)
}

func (b *Builder) TelegramRefresh(ctx context.Context) (TelegramScreen, error) {
	return b.telegram(ctx, nil, nil, "", true)
}

// TelegramEditKeys — «Изменить api_id и api_hash» (ревью M2, I1).
func (b *Builder) TelegramEditKeys(ctx context.Context) (TelegramScreen, error) {
	b.d.Login.EditKeys()
	return b.telegram(ctx, nil, nil, "", false)
}

func (b *Builder) telegram(ctx context.Context, input, problems map[string]string, message string, refresh bool) (TelegramScreen, error) {
	st, err := b.d.Login.State(ctx)
	if err != nil {
		return TelegramScreen{}, fmt.Errorf("состояние входа в Telegram: %w", err)
	}
	s := TelegramScreen{Title: texts.TgTitle, SubmitLabel: texts.TgNext, SavingLabel: texts.Saving, Message: message,
		GroupsTitle: texts.TgGroupsTitle, RefreshLabel: texts.TgRefresh, Fields: []Field{}, Groups: []GroupRow{}}
	b.botSection(&s)
	if st.Step != tgservice.StepKeys {
		s.EditKeysLabel = texts.TgEditKeys
	}
	switch st.Step {
	case tgservice.StepOffline:
		s.Message, s.GroupsTitle = texts.TgNoConnection, ""
		return s, nil
	case tgservice.StepReady:
	default:
		s.Hint, s.Fields = loginStep(st, input, problems)
		return s, nil
	}
	s.Status = fmt.Sprintf(texts.TgLoggedIn, st.Name)
	views, err := b.d.Groups.List(ctx, refresh)
	if err != nil {
		return TelegramScreen{}, fmt.Errorf("группы Telegram: %w", err)
	}
	if len(views) == 0 {
		s.GroupsEmpty = texts.TgGroupsEmpty
	}
	for _, v := range views {
		s.Groups = append(s.Groups, groupRow(v))
	}
	return s, nil
}

func loginStep(st tgservice.LoginState, input, problems map[string]string) (string, []Field) {
	field := func(key, label, kind string) Field {
		value := input[key]
		if kind == "password" {
			value = "" // пароль в форму не возвращается
		}
		return Field{Key: key, Label: label, Value: value, Kind: kind, Error: problems[key]}
	}
	switch st.Step {
	case tgservice.StepKeys:
		return texts.TgKeysHint, []Field{field(tgservice.FieldAPIID, texts.TgAPIID, "number"),
			field(tgservice.FieldAPIHash, texts.TgAPIHash, "text")}
	case tgservice.StepCode:
		return fmt.Sprintf(texts.TgCodeHint, st.Phone), []Field{field(tgservice.FieldCode, texts.TgCode, "text")}
	case tgservice.StepPassword:
		return texts.TgPasswordHint, []Field{field(tgservice.FieldPassword, texts.TgPassword, "password")}
	default:
		return texts.TgPhoneHint, []Field{field(tgservice.FieldPhone, texts.TgPhone, "text")}
	}
}

func groupRow(v tgservice.GroupView) GroupRow {
	row := GroupRow{Key: v.Group.Peer.Key(), Title: v.Group.Title}
	if v.Added {
		row.Status = texts.TgAdded
		return row
	}
	row.Action = &Action{ID: "add", Label: texts.TgAddGroup, Tone: "primary"}
	return row
}

// TelegramBot — сохранить токен бота; ошибка формата — у поля.
func (b *Builder) TelegramBot(ctx context.Context, token string) (TelegramScreen, error) {
	problem, err := b.d.Bot.SetToken(ctx, token)
	if err != nil {
		return TelegramScreen{}, fmt.Errorf("токен бота: %w", err)
	}
	if problem != "" {
		s, err := b.telegram(ctx, nil, nil, "", false)
		s.BotField.Error = problem
		return s, err
	}
	return b.telegram(ctx, nil, nil, texts.TgBotSaved, false)
}

func (b *Builder) botSection(s *TelegramScreen) {
	s.BotTitle, s.BotHint, s.BotSaveLabel = texts.TgBotTitle, texts.TgBotHint, texts.TgBotSave
	s.BotField = Field{Key: tgservice.FieldBotToken, Label: texts.TgBotToken, Kind: "password"}
	st := b.d.Bot.Status()
	switch st.State {
	case tgservice.BotRunning:
		s.BotStatus = fmt.Sprintf(texts.TgBotRunning, st.Username)
	case tgservice.BotStarting:
		s.BotStatus = texts.TgBotStarting
	case tgservice.BotBadToken:
		s.BotStatus = texts.TgBotBadToken
	case tgservice.BotConflict:
		s.BotStatus = texts.TgBotConflict
	case tgservice.BotOffline:
		s.BotStatus = texts.TgBotOffline
	default:
		s.BotStatus = texts.TgBotOff
	}
}
