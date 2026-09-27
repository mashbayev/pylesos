// Файл: internal/screens/people_test.go
package screens

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	pdomain "pylesos/internal/people/domain"
	"pylesos/internal/texts"
)

type fakePeople struct {
	list    []pdomain.Summary
	persons map[int64]pdomain.Person
	views   []pdomain.MentionView
	loose   []pdomain.MentionView
	cands   []pdomain.Candidate
	byKey   []pdomain.Person
	did     string
}

func (f *fakePeople) Search(_ context.Context, q string, _ int) ([]pdomain.Summary, error) {
	f.did = "search:" + q
	return f.list, nil
}
func (f *fakePeople) Person(_ context.Context, id int64) (pdomain.Person, error) {
	if p, ok := f.persons[id]; ok {
		return p, nil
	}
	return pdomain.Person{}, pdomain.ErrNotFound
}
func (f *fakePeople) PersonMentions(context.Context, int64) ([]pdomain.MentionView, error) {
	return f.views, nil
}
func (f *fakePeople) UnlinkedViews(context.Context) ([]pdomain.MentionView, error) {
	return f.loose, nil
}
func (f *fakePeople) Candidates(context.Context) ([]pdomain.Candidate, error) { return f.cands, nil }
func (f *fakePeople) PersonsByKey(context.Context, string) ([]pdomain.Person, error) {
	return f.byKey, nil
}
func (f *fakePeople) Merge(_ context.Context, a, b int64) error {
	f.did = fmt.Sprintf("merge:%d:%d", a, b)
	return nil
}
func (f *fakePeople) Reject(_ context.Context, a, b int64) error {
	f.did = fmt.Sprintf("reject:%d:%d", a, b)
	return nil
}
func (f *fakePeople) Link(_ context.Context, m, p int64, name string) error {
	f.did = fmt.Sprintf("link:%d:%d:%s", m, p, name)
	return nil
}

func peopleBuilder(f *fakePeople) *Builder {
	return NewBuilder(Deps{Jobs: fakeJobs{}, Kinds: fakeKinds{}, Settings: &fakeSettings{}, Sources: fakeSources{},
		Counts: fakeCounts{}, Stats: fakeStats{}, Login: &fakeLogin{}, Groups: &fakeGroups{}, Bot: &fakeBot{},
		Forms: &fakeForms{}, Ollama: fakeOllama{}, People: f, Loc: time.UTC})
}

func iin(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

func TestPeopleListAndCard(t *testing.T) {
	ivan := pdomain.Person{ID: 1, FullName: "Тестов Иван", IIN: iin("900101300007")}
	f := &fakePeople{list: []pdomain.Summary{{Person: ivan, Records: 2, Comments: 1, Groups: "ЧС Тест"}},
		persons: map[int64]pdomain.Person{1: ivan},
		views: []pdomain.MentionView{{Mention: pdomain.Mention{MsgID: 7, Role: pdomain.RoleRecord}, Date: time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC).Unix(),
			Text: "Тестов Иван ЧС!", SourceTitle: "ЧС Тест", SourceCfg: `{"type":"channel","id":123,"access_hash":1}`}},
		cands: []pdomain.Candidate{{A: 1, B: 1}}}
	s, err := peopleBuilder(f).People(context.Background(), PeopleQuery{Query: "иван", Person: 1})
	if err != nil {
		t.Fatal(err)
	}
	if f.did != "search:иван" || s.EmptyText != "" || len(s.Rows) != 1 || s.Rows[0].Lines[0] != fmt.Sprintf(texts.PeopleCounts, 2, 0, 1) || len(s.Tabs) != 3 || s.Tabs[1].Label != texts.PeopleTabCands+" (1)" {
		t.Fatalf("список: %+v", s)
	}
	c := s.Card
	if c == nil || c.Name != "Тестов Иван" || c.Fields[0].Value != "900101300007" || len(c.Mentions) != 1 ||
		c.Mentions[0].Link != "https://t.me/c/123/7" || c.Mentions[0].Role != texts.RoleRecord || c.Mentions[0].Date != "20.09.2026 10:00" {
		t.Fatalf("карточка: %+v", c)
	}
}

func TestPeopleCandidatesAndUnlinkedActions(t *testing.T) {
	f := &fakePeople{persons: map[int64]pdomain.Person{1: {ID: 1, FullName: "Тестов Иван"}, 2: {ID: 2, FullName: "Тестов Иван Петрович", IIN: iin("900101300007")}},
		cands: []pdomain.Candidate{{A: 1, B: 2}},
		loose: []pdomain.MentionView{{Mention: pdomain.Mention{ID: 9, NameAsWritten: "Тестов Иван"}, Text: "Тестов Иван есть инфо?"}},
		byKey: []pdomain.Person{{ID: 1, FullName: "Тестов Иван"}, {ID: 3, FullName: "Тестов Иван"}}}
	b := peopleBuilder(f)
	if s, err := b.People(context.Background(), PeopleQuery{Tab: "unlinked", Query: "x"}); err != nil || s.EmptyText != "" {
		t.Fatalf("не пусто: %+v %v", s, err)
	}
	f.loose, f.cands = nil, nil
	if s, err := b.People(context.Background(), PeopleQuery{Tab: "candidates"}); err != nil || s.EmptyText != texts.PeopleCandsEmpty {
		t.Fatalf("пусто: %+v %v", s, err)
	}
	f.loose = []pdomain.MentionView{{Mention: pdomain.Mention{ID: 9, NameAsWritten: "Тестов Иван"}, Text: "Тестов Иван есть инфо?"}}
	f.cands = []pdomain.Candidate{{A: 1, B: 2}}
	s, err := b.People(context.Background(), PeopleQuery{Tab: "candidates"})
	if err != nil || len(s.Candidates) != 1 || s.Candidates[0].B.Name != "Тестов Иван Петрович" {
		t.Fatalf("кандидаты: %+v %v", s.Candidates, err)
	}
	if s, err = b.PeopleMerge(context.Background(), 1, 2); err != nil || f.did != "merge:1:2" || s.Message != texts.PeopleMerged {
		t.Fatalf("склейка: %s %+v %v", f.did, s.Message, err)
	}
	s, err = b.People(context.Background(), PeopleQuery{Tab: "unlinked"})
	if err != nil || len(s.Unlinked) != 1 || len(s.Unlinked[0].Options) != 3 || s.Unlinked[0].Options[2].Label != fmt.Sprintf(texts.PeopleNewPerson, "Тестов Иван") {
		t.Fatalf("не привязано: %+v %v", s.Unlinked, err)
	}
	if _, err := b.PeopleLink(context.Background(), 9, 0); err != nil || f.did != "link:9:0:Тестов Иван" {
		t.Fatalf("новый человек: %s %v", f.did, err)
	}
}
