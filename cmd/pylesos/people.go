// Файл: cmd/pylesos/people.go
package main

import (
	"context"
	"fmt"
	"os"

	"pylesos/internal/analyze"
	"pylesos/internal/export"
	jobsservice "pylesos/internal/jobs/service"
	prepo "pylesos/internal/people/repository"
	pservice "pylesos/internal/people/service"
	tservice "pylesos/internal/table/service"
	"pylesos/internal/tables"
	tgservice "pylesos/internal/telegram/service"
)

// peopleStore — экрану «Люди» чтение из репозитория, действия — через сервис.
type peopleStore struct {
	*prepo.SQLite
	svc *pservice.Service
}

func (p peopleStore) Merge(ctx context.Context, a, b int64) error  { return p.svc.Merge(ctx, a, b) }
func (p peopleStore) Reject(ctx context.Context, a, b int64) error { return p.svc.Reject(ctx, a, b) }
func (p peopleStore) Link(ctx context.Context, mentionID, personID int64, name string) error {
	return p.svc.Link(ctx, mentionID, personID, name)
}

// sourceActions — кнопки источника: «Собрать» (группы Telegram) и «Разобрать» (задание tg_analyze).
type sourceActions struct {
	*tgservice.Groups
	runner *jobsservice.Runner
}

func (s sourceActions) Analyze(ctx context.Context, sourceID int64) error {
	_, err := s.runner.Start(ctx, sourceID, analyze.KindAnalyze, "{}")
	return err
}

// Export — «В Excel»: задание выгрузки людей группы.
func (s sourceActions) Export(ctx context.Context, sourceID int64) error {
	_, err := s.runner.Start(ctx, sourceID, tservice.KindExport, "{}")
	return err
}

// writeExcel — книга в папку выгрузок (папка создаётся, если её нет).
func writeExcel(dir string, tpl tables.Template, rows []tables.Row) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("папка выгрузок: %w", err)
	}
	return export.WriteExcel(dir, tpl, rows, nil)
}
