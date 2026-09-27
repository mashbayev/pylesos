// Файл: internal/screens/home.go
package screens

import (
	"context"
	"fmt"

	jobsdomain "pylesos/internal/jobs/domain"
	sourcesdomain "pylesos/internal/sources/domain"
	"pylesos/internal/texts"
)

// Home — главная: плашки «ждёт вас» и группы Telegram.
func (b *Builder) Home(ctx context.Context) (HomeScreen, error) {
	banners, err := b.banners(ctx)
	if err != nil {
		return HomeScreen{}, err
	}
	rows, err := b.telegramRows(ctx)
	if err != nil {
		return HomeScreen{}, fmt.Errorf("экран «Источники»: %w", err)
	}
	empty := texts.HomeEmpty
	if len(rows) > 0 {
		empty = ""
	}
	return HomeScreen{Title: texts.HomeTitle, Banners: banners, EmptyText: empty,
		TelegramTitle: texts.HomeTelegramTitle, Telegram: rows}, nil
}

func (b *Builder) banners(ctx context.Context) ([]Banner, error) {
	items, err := b.d.Jobs.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("экран «Источники»: %w", err)
	}
	banners := []Banner{}
	for _, it := range items {
		if it.Status == jobsdomain.StatusWaitingUser {
			text := fmt.Sprintf(texts.BannerWaiting, it.SourceTitle, b.d.Kinds.KindTitle(it.Kind), it.Reason)
			banners = append(banners, Banner{JobID: it.ID, Text: text})
		}
	}
	return banners, nil
}

func (b *Builder) telegramRows(ctx context.Context) ([]SourceRow, error) {
	sources, err := b.d.Sources.ListByKind(ctx, sourcesdomain.KindTelegramGroup)
	if err != nil {
		return nil, err
	}
	counts, err := b.d.Counts.Counts(ctx)
	if err != nil {
		return nil, err
	}
	stats, err := b.d.Stats.SourceStats(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]SourceRow, 0, len(sources))
	for _, s := range sources {
		rows = append(rows, b.sourceRow(s, counts[s.ID], stats[s.ID]))
	}
	return rows, nil
}

func (b *Builder) sourceRow(s sourcesdomain.Source, count int, st jobsdomain.SourceStats) SourceRow {
	last := texts.TgNeverSynced
	if st.LastDone > 0 {
		last = fmt.Sprintf(texts.TgLastSync, b.formatTime(st.LastDone))
	}
	actions := []Action{}
	if !st.Active {
		actions = append(actions, Action{ID: "collect", Label: texts.TgCollect, Tone: "primary"},
			Action{ID: "analyze", Label: texts.AnalyzeCollect, Tone: "default"},
			Action{ID: "export", Label: texts.TableExport, Tone: "default"})
	}
	return SourceRow{ID: s.ID, Title: s.Title, Lines: []string{fmt.Sprintf(texts.TgMessagesCount, count), last}, Actions: actions}
}
