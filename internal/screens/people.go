// Файл: internal/screens/people.go
package screens

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	pdomain "pylesos/internal/people/domain"
	"pylesos/internal/people/names"
	pservice "pylesos/internal/people/service"
	tgdomain "pylesos/internal/telegram/domain"
	"pylesos/internal/texts"
)

// PeopleStore — люди: чтение (people/repository) и действия (people/service).
type PeopleStore interface {
	Search(ctx context.Context, query string, limit int) ([]pdomain.Summary, error)
	Person(ctx context.Context, id int64) (pdomain.Person, error)
	PersonMentions(ctx context.Context, personID int64) ([]pdomain.MentionView, error)
	UnlinkedViews(ctx context.Context) ([]pdomain.MentionView, error)
	Candidates(ctx context.Context) ([]pdomain.Candidate, error)
	PersonsByKey(ctx context.Context, key string) ([]pdomain.Person, error)
	Merge(ctx context.Context, a, b int64) error
	Reject(ctx context.Context, a, b int64) error
	Link(ctx context.Context, mentionID, personID int64, name string) error
}

// PeopleQuery — что открыть: вкладка ("", "candidates", "unlinked"), поиск, человек.
type PeopleQuery struct {
	Tab    string
	Query  string
	Person int64
}

const peopleLimit = 200

func (b *Builder) People(ctx context.Context, q PeopleQuery) (PeopleScreen, error) {
	return b.people(ctx, q, "")
}

func (b *Builder) PeopleMerge(ctx context.Context, a, c int64) (PeopleScreen, error) {
	return b.afterPeople(ctx, PeopleQuery{Tab: "candidates"}, b.d.People.Merge(ctx, a, c), texts.PeopleMerged)
}

func (b *Builder) PeopleReject(ctx context.Context, a, c int64) (PeopleScreen, error) {
	return b.afterPeople(ctx, PeopleQuery{Tab: "candidates"}, b.d.People.Reject(ctx, a, c), texts.PeopleRejected)
}

// PeopleLink — привязать упоминание; personID 0 — новый человек с именем из упоминания.
func (b *Builder) PeopleLink(ctx context.Context, mentionID, personID int64) (PeopleScreen, error) {
	name, err := b.mentionName(ctx, mentionID)
	if err == nil {
		err = b.d.People.Link(ctx, mentionID, personID, name)
	}
	return b.afterPeople(ctx, PeopleQuery{Tab: "unlinked"}, err, texts.PeopleLinked)
}

func (b *Builder) mentionName(ctx context.Context, mentionID int64) (string, error) {
	loose, err := b.d.People.UnlinkedViews(ctx)
	if err != nil {
		return "", err
	}
	for _, m := range loose {
		if m.ID == mentionID {
			return m.NameAsWritten, nil
		}
	}
	return "", pdomain.ErrNotFound
}

func (b *Builder) afterPeople(ctx context.Context, q PeopleQuery, err error, done string) (PeopleScreen, error) {
	switch {
	case err == nil:
		return b.people(ctx, q, done)
	case errors.Is(err, pservice.ErrNoName):
		return b.people(ctx, q, texts.ErrPeopleNoName)
	case errors.Is(err, pdomain.ErrNotFound):
		return b.people(ctx, q, texts.ErrPeopleNotFound)
	}
	return PeopleScreen{}, fmt.Errorf("люди: %w", err)
}

func (b *Builder) people(ctx context.Context, q PeopleQuery, message string) (PeopleScreen, error) {
	cands, err := b.d.People.Candidates(ctx)
	if err != nil {
		return PeopleScreen{}, fmt.Errorf("люди: %w", err)
	}
	loose, err := b.d.People.UnlinkedViews(ctx)
	if err != nil {
		return PeopleScreen{}, fmt.Errorf("люди: %w", err)
	}
	s := PeopleScreen{Title: texts.PeopleTitle, Tab: q.Tab, Query: q.Query, SearchLabel: texts.PeopleSearch,
		Message: message, OpenLabel: texts.PeopleOpen, Tabs: []NavItem{
			{ID: "", Label: texts.PeopleTabAll},
			{ID: "candidates", Label: counted(texts.PeopleTabCands, len(cands))},
			{ID: "unlinked", Label: counted(texts.PeopleTabUnlinked, len(loose))},
		}}
	switch q.Tab {
	case "candidates":
		s.EmptyText = texts.PeopleCandsEmpty
		s.Candidates, err = b.candidateRows(ctx, cands)
	case "unlinked":
		s.EmptyText = texts.PeopleLooseEmpty
		s.Unlinked, err = b.unlinkedRows(ctx, loose)
	default:
		s.Tab = ""
		s.EmptyText = texts.PeopleEmpty
		err = b.peopleList(ctx, q, &s)
	}
	if err != nil {
		return PeopleScreen{}, fmt.Errorf("люди: %w", err)
	}
	if len(s.Rows)+len(s.Candidates)+len(s.Unlinked) > 0 {
		s.EmptyText = "" // текст «пусто» — только для пустой вкладки
	}
	return s, nil
}

func counted(label string, n int) string {
	if n == 0 {
		return label
	}
	return fmt.Sprintf("%s (%d)", label, n)
}

func (b *Builder) peopleList(ctx context.Context, q PeopleQuery, s *PeopleScreen) error {
	list, err := b.d.People.Search(ctx, q.Query, peopleLimit)
	if err != nil {
		return err
	}
	for _, p := range list {
		s.Rows = append(s.Rows, SourceRow{ID: p.ID, Title: p.FullName, Lines: summaryLines(p)})
	}
	if q.Person == 0 {
		return nil
	}
	p, err := b.d.People.Person(ctx, q.Person)
	if errors.Is(err, pdomain.ErrNotFound) {
		s.Message = texts.ErrPeopleNotFound
		return nil
	}
	if err != nil {
		return err
	}
	views, err := b.d.People.PersonMentions(ctx, p.ID)
	if err != nil {
		return err
	}
	card := &PersonCard{ID: p.ID, Name: p.FullName, Fields: []Field{
		{Key: "iin", Label: texts.PeopleIIN, Value: orUnknown(p.IIN.String)},
		{Key: "doc_number", Label: texts.PeopleDoc, Value: orUnknown(p.DocNumber)},
		{Key: "birth_date", Label: texts.PeopleBirth, Value: orUnknown(p.BirthDate)},
	}}
	for _, v := range views {
		card.Mentions = append(card.Mentions, b.mentionRow(v))
	}
	s.Card = card
	return nil
}

func summaryLines(p pdomain.Summary) []string {
	lines := []string{fmt.Sprintf(texts.PeopleCounts, p.Records, p.Queries, p.Comments)}
	if p.IIN.Valid {
		lines = append(lines, texts.PeopleIIN+": "+p.IIN.String)
	}
	if p.Groups != "" {
		lines = append(lines, fmt.Sprintf(texts.PeopleGroups, p.Groups))
	}
	return lines
}

func orUnknown(v string) string {
	if v == "" {
		return texts.PeopleUnknown
	}
	return v
}

func (b *Builder) mentionRow(v pdomain.MentionView) MentionRow {
	var peer tgdomain.Peer
	link := ""
	if json.Unmarshal([]byte(v.SourceCfg), &peer) == nil {
		link = peer.MessageLink(v.MsgID)
	}
	return MentionRow{Role: roleText(v.Role), Date: b.formatTime(v.Date), Group: v.SourceTitle, Author: v.Author, Text: v.Text, Link: link}
}

func roleText(r pdomain.Role) string {
	switch r {
	case pdomain.RoleRecord:
		return texts.RoleRecord
	case pdomain.RoleQuery:
		return texts.RoleQuery
	case pdomain.RoleComment:
		return texts.RoleComment
	}
	return ""
}

func (b *Builder) candidateRows(ctx context.Context, cands []pdomain.Candidate) ([]CandidateRow, error) {
	var rows []CandidateRow
	for _, c := range cands {
		a, err := b.brief(ctx, c.A)
		if err != nil {
			return nil, err
		}
		o, err := b.brief(ctx, c.B)
		if err != nil {
			return nil, err
		}
		rows = append(rows, CandidateRow{A: a, B: o, Actions: []Action{
			{ID: "merge", Label: texts.PeopleMerge, Tone: "primary"},
			{ID: "reject", Label: texts.PeopleReject, Tone: "default"},
		}})
	}
	return rows, nil
}

func (b *Builder) brief(ctx context.Context, id int64) (PersonBrief, error) {
	p, err := b.d.People.Person(ctx, id)
	if err != nil {
		return PersonBrief{}, err
	}
	out := PersonBrief{ID: p.ID, Name: p.FullName}
	if p.IIN.Valid {
		out.Lines = append(out.Lines, texts.PeopleIIN+": "+p.IIN.String)
	}
	if p.BirthDate != "" {
		out.Lines = append(out.Lines, texts.PeopleBirth+": "+p.BirthDate)
	}
	return out, nil
}

func (b *Builder) unlinkedRows(ctx context.Context, loose []pdomain.MentionView) ([]UnlinkedRow, error) {
	var rows []UnlinkedRow
	for _, v := range loose {
		row := UnlinkedRow{ID: v.ID, Mention: b.mentionRow(v), Name: v.NameAsWritten, LinkLabel: texts.PeopleLink}
		if key := names.Key(v.NameAsWritten); key != "" {
			same, err := b.d.People.PersonsByKey(ctx, key)
			if err != nil {
				return nil, err
			}
			for _, p := range same {
				label := p.FullName
				if p.IIN.Valid {
					label += ", " + texts.PeopleIIN + " " + p.IIN.String
				}
				row.Options = append(row.Options, Option{ID: p.ID, Label: label})
			}
		}
		if v.NameAsWritten != "" {
			row.Options = append(row.Options, Option{ID: 0, Label: fmt.Sprintf(texts.PeopleNewPerson, v.NameAsWritten)})
		}
		rows = append(rows, row)
	}
	return rows, nil
}
