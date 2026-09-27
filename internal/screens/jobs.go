// Файл: internal/screens/jobs.go
package screens

import (
	"context"
	"fmt"

	jobsdomain "pylesos/internal/jobs/domain"
	"pylesos/internal/texts"
)

type statusView struct{ text, tone string }

func statusOf(s jobsdomain.Status) statusView {
	switch s {
	case jobsdomain.StatusQueued:
		return statusView{texts.StatusQueued, "neutral"}
	case jobsdomain.StatusRunning:
		return statusView{texts.StatusRunning, "running"}
	case jobsdomain.StatusPaused:
		return statusView{texts.StatusPaused, "neutral"}
	case jobsdomain.StatusWaitingUser:
		return statusView{texts.StatusWaitingUser, "warning"}
	case jobsdomain.StatusDone:
		return statusView{texts.StatusDone, "success"}
	case jobsdomain.StatusFailed:
		return statusView{texts.StatusFailed, "danger"}
	default:
		return statusView{texts.StatusCancelled, "neutral"}
	}
}

func actionOf(a jobsdomain.Action) Action {
	switch a {
	case jobsdomain.ActionPause:
		return Action{ID: string(a), Label: texts.ActionPause, Tone: "default"}
	case jobsdomain.ActionResume:
		return Action{ID: string(a), Label: texts.ActionResume, Tone: "primary"}
	default:
		return Action{ID: string(a), Label: texts.ActionCancel, Tone: "danger"}
	}
}

func (b *Builder) Jobs(ctx context.Context) (JobsScreen, error) {
	items, err := b.d.Jobs.List(ctx)
	if err != nil {
		return JobsScreen{}, fmt.Errorf("экран «Задания»: %w", err)
	}
	rows := make([]JobRow, 0, len(items))
	for _, it := range items {
		rows = append(rows, b.jobRow(it))
	}
	return JobsScreen{Title: texts.JobsTitle, EmptyText: texts.JobsEmpty, Jobs: rows}, nil
}

func (b *Builder) jobRow(it jobsdomain.ListItem) JobRow {
	st := statusOf(it.Status)
	actions := []Action{}
	for _, a := range it.Status.Actions() {
		actions = append(actions, actionOf(a))
	}
	return JobRow{
		ID:           it.ID,
		Title:        fmt.Sprintf(texts.JobTitle, b.d.Kinds.KindTitle(it.Kind), it.SourceTitle),
		StatusText:   st.text,
		StatusTone:   st.tone,
		ProgressText: it.ProgressText,
		Reason:       it.Reason,
		UpdatedText:  b.formatTime(it.UpdatedAt),
		Actions:      actions,
	}
}
