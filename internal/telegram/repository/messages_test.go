// Файл: internal/telegram/repository/messages_test.go
package repository

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"

	"pylesos/internal/store/storetest"
	"pylesos/internal/telegram/domain"
)

func msg(id int, text string) domain.Message {
	return domain.Message{ID: id, Date: int64(1000 + id), AuthorID: 7, AuthorName: "Тестовый Автор", Text: text}
}

func textOf(t *testing.T, db *sqlx.DB, src int64, id int) string {
	t.Helper()
	var s string
	if err := db.Get(&s, `SELECT text FROM tg_messages WHERE source_id = ? AND msg_id = ?`, src, id); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSaveBoundsCountsAndMedia(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	repo := NewSQLite(db)
	photo := msg(3, "Фото документа")
	photo.Media = []domain.Media{{Kind: domain.MediaPhoto, FileID: 55, MimeType: "image/jpeg"}}
	if err := repo.Save(ctx, src, []domain.Message{msg(5, "пять"), photo, msg(4, "четыре")}, ViaAccount, 2000); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, src, []domain.Message{photo}, ViaAccount, 2001); err != nil {
		t.Fatalf("повторная запись с вложением: %v", err)
	}
	minID, maxID, err := repo.Bounds(ctx, src)
	if err != nil || minID != 3 || maxID != 5 {
		t.Fatalf("границы %d..%d %v", minID, maxID, err)
	}
	counts, err := repo.Counts(ctx)
	if err != nil || counts[src] != 3 {
		t.Fatalf("число сообщений: %v %v", counts, err)
	}
	var media int
	if err := db.Get(&media, `SELECT COUNT(*) FROM tg_media WHERE source_id = ? AND status = 'pending'`, src); err != nil || media != 1 {
		t.Fatalf("вложений %d %v", media, err)
	}
	if lo, hi, err := repo.Bounds(ctx, 999); err != nil || lo != 0 || hi != 0 {
		t.Fatalf("пустая группа: %d..%d %v", lo, hi, err)
	}
}

func TestResaveDoesNotEraseAndOlderEditDoesNotOverwrite(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	repo := NewSQLite(db)
	edited := msg(1, "Исправленный текст")
	edited.EditDate = 5000
	if err := repo.Save(ctx, src, []domain.Message{edited}, ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	stale := msg(1, "Старый текст") // пришла версия до правки
	if err := repo.Save(ctx, src, []domain.Message{stale}, ViaAccount, 2); err != nil {
		t.Fatal(err)
	}
	if got := textOf(t, db, src, 1); got != "Исправленный текст" {
		t.Fatalf("старая версия затёрла правку: %q", got)
	}
	empty := domain.Message{ID: 1, Date: 1001, EditDate: 6000} // без автора и текста
	if err := repo.Save(ctx, src, []domain.Message{empty}, ViaAccount, 3); err != nil {
		t.Fatal(err)
	}
	var author string
	if err := db.Get(&author, `SELECT author_name FROM tg_messages WHERE source_id = ? AND msg_id = 1`, src); err != nil {
		t.Fatal(err)
	}
	if author != "Тестовый Автор" || textOf(t, db, src, 1) != "Исправленный текст" {
		t.Fatalf("пустые поля затёрли сохранённое: автор %q, текст %q", author, textOf(t, db, src, 1))
	}
}

func TestMarkDeletedAndResaveRestores(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	repo := NewSQLite(db)
	if err := repo.Save(ctx, src, []domain.Message{msg(1, "а"), msg(2, "б"), msg(3, "в")}, ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkDeleted(ctx, src, []int{2}); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkDeleted(ctx, src, nil); err != nil {
		t.Fatalf("пустой список: %v", err)
	}
	ids, err := repo.LiveIDsAfter(ctx, src, 0, 10)
	if err != nil || len(ids) != 2 || ids[0] != 1 || ids[1] != 3 {
		t.Fatalf("живые: %v %v", ids, err)
	}
	if ids, err := repo.LiveIDsAfter(ctx, src, 1, 1); err != nil || len(ids) != 1 || ids[0] != 3 {
		t.Fatalf("после 1, по одному: %v %v", ids, err)
	}
	counts, err := repo.Counts(ctx)
	if err != nil || counts[src] != 2 {
		t.Fatalf("удалённое посчитано: %v %v", counts, err)
	}
	if err := repo.Save(ctx, src, []domain.Message{msg(2, "б")}, ViaAccount, 2); err != nil {
		t.Fatal(err)
	}
	if counts, err := repo.Counts(ctx); err != nil || counts[src] != 3 {
		t.Fatalf("снова загруженное не восстановлено: %v %v", counts, err)
	}
}

// Отметка сверки аккаунтом живёт у источника, а не вычисляется из MAX(msg_id) (ревью M2: C1).
func TestSyncStateAndRecentWindow(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	repo := NewSQLite(db)
	st, err := repo.SyncState(ctx, src)
	if err != nil || st != (SyncState{}) {
		t.Fatalf("новая группа: %+v %v", st, err)
	}
	want := SyncState{SyncedTop: 100, PassHead: 150, PassLow: 120}
	if err := repo.SaveSyncState(ctx, src, want); err != nil {
		t.Fatal(err)
	}
	if st, err := repo.SyncState(ctx, src); err != nil || st != want {
		t.Fatalf("сохранено %+v, прочитано %+v %v", want, st, err)
	}
	if err := repo.Save(ctx, src, []domain.Message{msg(1, "а"), msg(2, "б"), msg(3, "в"), msg(4, "г")}, ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkDeleted(ctx, src, []int{3}); err != nil {
		t.Fatal(err)
	}
	if after, err := repo.RecentWindowStart(ctx, src, 2); err != nil || after != 1 {
		t.Fatalf("окно из 2 последних живых (4, 2) начинается после 1: %d %v", after, err)
	}
	if after, err := repo.RecentWindowStart(ctx, src, 10); err != nil || after != 0 {
		t.Fatalf("окно больше истории: %d %v", after, err)
	}
}

// M4: разобранное сообщение после новой правки снова ждёт разбора; порции неразобранных — по порядку.
func TestAnalyzedResetOnEdit(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	repo := NewSQLite(db)
	if err := repo.Save(ctx, src, []domain.Message{msg(1, "а"), msg(2, "б")}, ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.PendingAnalysis(ctx, src, 10)
	if err != nil || len(pending) != 2 || pending[0].ID != 1 {
		t.Fatalf("ждут разбора: %+v %v", pending, err)
	}
	if err := repo.MarkAnalyzed(ctx, src, 1, ""); err != nil {
		t.Fatal(err)
	}
	if err := repo.MarkAnalyzed(ctx, src, 2, "ошибка модели"); err != nil {
		t.Fatal(err)
	}
	if pending, err := repo.PendingAnalysis(ctx, src, 10); err != nil || len(pending) != 0 {
		t.Fatalf("после разбора: %+v %v", pending, err)
	}
	same := msg(1, "а") // пришло снова без правки — разбор не повторять
	if err := repo.Save(ctx, src, []domain.Message{same}, ViaBot, 2); err != nil {
		t.Fatal(err)
	}
	edited := msg(1, "а — исправлено")
	edited.EditDate = 500
	if err := repo.Save(ctx, src, []domain.Message{edited}, ViaBot, 3); err != nil {
		t.Fatal(err)
	}
	pending, err = repo.PendingAnalysis(ctx, src, 10)
	if err != nil || len(pending) != 1 || pending[0].ID != 1 || pending[0].Text != "а — исправлено" {
		t.Fatalf("после правки: %+v %v", pending, err)
	}
}

// M4: вложения ждут распознавания; результат (текст и поля) сохраняется, статус — «распознано».
func TestMediaRecognitionQueue(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	src := storetest.AddSource(t, db, "Группа")
	repo := NewSQLite(db)
	photo := msg(4, "подпись")
	photo.Media = []domain.Media{{Kind: domain.MediaPhoto, MimeType: "image/jpeg"}}
	if err := repo.Save(ctx, src, []domain.Message{msg(3, "без вложений"), photo}, ViaAccount, 1); err != nil {
		t.Fatal(err)
	}
	pending, err := repo.PendingMedia(ctx, src, 10)
	if err != nil || len(pending) != 1 || pending[0].MsgID != 4 || pending[0].Kind != domain.MediaPhoto {
		t.Fatalf("ждут распознавания: %+v %v", pending, err)
	}
	if err := repo.SaveRecognition(ctx, src, 4, 0, "текст документа", `{"iin":"900101300011"}`, "gemma4:e4b"); err != nil {
		t.Fatal(err)
	}
	if pending, err := repo.PendingMedia(ctx, src, 10); err != nil || len(pending) != 0 {
		t.Fatalf("после распознавания: %+v %v", pending, err)
	}
	fields, err := repo.MediaFields(ctx, src, 4)
	if err != nil || len(fields) != 1 || fields[0]["iin"] != "900101300011" {
		t.Fatalf("поля: %+v %v", fields, err)
	}
}
