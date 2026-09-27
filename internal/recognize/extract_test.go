// Файл: internal/recognize/extract_test.go
package recognize

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"pylesos/internal/llm"
	"pylesos/internal/normalize"
	"pylesos/internal/tables"
)

// queueCore отвечает по очереди заготовленными ответами и запоминает запросы.
type queueCore struct {
	replies []string
	reqs    []llm.Request
}

func (q *queueCore) Respond(_ context.Context, r llm.Request) (string, error) {
	q.reqs = append(q.reqs, r)
	if len(q.replies) == 0 {
		return "", errors.New("нет заготовленного ответа")
	}
	out := q.replies[0]
	q.replies = q.replies[1:]
	return out, nil
}

var driverTpl = tables.Template{Name: "Водители", Mode: tables.ModeDocument, Columns: []tables.Column{
	{Name: "ФИО", Type: tables.TypeText},
	{Name: "ИИН", Type: tables.TypeIIN},
}}

func imgPages(n int) []PageText {
	out := make([]PageText, n)
	for i := range out {
		out[i] = PageText{Page: normalize.Page{Source: "a.pdf", Index: i + 1, Image: []byte{byte(i)}}, Text: "текст"}
	}
	return out
}

func TestReviewSendsImageAndTranscript(t *testing.T) {
	q := &queueCore{replies: []string{"Үміт"}}
	got, err := (&Transcriber{LLM: q, Model: "m"}).Review(context.Background(), imgPage, "Уміт")
	if err != nil || got != "Үміт" {
		t.Fatalf("got %q err %v", got, err)
	}
	r := q.reqs[0]
	if r.System != reviewPrompt || len(r.Images) != 1 || !strings.Contains(r.Text, "Уміт") {
		t.Fatalf("req %+v", r)
	}
}

func TestReviewTextOnlyPageSkipsCore(t *testing.T) {
	q := &queueCore{}
	got, err := (&Transcriber{LLM: q, Model: "m"}).Review(context.Background(), normalize.Page{TextLayer: "x"}, "мәтін")
	if err != nil || got != "мәтін" || len(q.reqs) != 0 {
		t.Fatalf("got %q err %v reqs %d", got, err, len(q.reqs))
	}
}

func TestExtractDocumentMergesChunks(t *testing.T) {
	q := &queueCore{replies: []string{
		`{"rows":[{"ФИО":{"value":"Әбдіхалықов Нұрсұлтан","confidence":"high"},"ИИН":{"value":"","confidence":"low"}}]}`,
		`{"rows":[{"ФИО":{"value":"Другой","confidence":"low"},"ИИН":{"value":"850315400124","confidence":"high"}}]}`,
	}}
	rows, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), driverTpl, "a.pdf", imgPages(12), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.reqs) != 2 || len(q.reqs[0].Images) != 10 || len(q.reqs[1].Images) != 2 {
		t.Fatalf("запросы: %d, картинки %d/%d", len(q.reqs), len(q.reqs[0].Images), len(q.reqs[len(q.reqs)-1].Images))
	}
	if len(rows) != 1 || rows[0].File != "a.pdf" || rows[0].Page != 0 {
		t.Fatalf("rows %+v", rows)
	}
	c := rows[0].Cells
	if c["ФИО"].Value != "Әбдіхалықов Нұрсұлтан" || c["ИИН"].Value != "850315400124" || c["ИИН"].Confidence != "high" {
		t.Fatalf("cells %+v", c)
	}
	if !strings.Contains(q.reqs[0].System, "ФИО") || !strings.Contains(q.reqs[0].System, "document") {
		t.Fatal("в системном промпте должно быть описание колонок и режим")
	}
}

func TestExtractTableRowsPerPage(t *testing.T) {
	tpl := tables.Template{Name: "Накладные", Mode: tables.ModeTableRows, Columns: []tables.Column{{Name: "Товар", Type: tables.TypeText}}}
	q := &queueCore{replies: []string{
		`{"rows":[{"Товар":"Ұн"},{"Товар":"Қант"}]}`,
		`{"rows":[{"Товар":"Май"}]}`,
	}}
	rows, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), tpl, "n.pdf", imgPages(2), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[0].Page != 1 || rows[1].Page != 1 || rows[2].Page != 2 || rows[2].Cells["Товар"].Value != "Май" {
		t.Fatalf("rows %+v", rows)
	}
	if len(q.reqs[0].Images) != 1 {
		t.Fatal("в режиме table_rows — по одной странице за запрос")
	}
}

func TestExtractPageModeExactlyOneRow(t *testing.T) {
	tpl := tables.Template{Name: "П", Mode: tables.ModePage, Columns: []tables.Column{{Name: "Номер", Type: tables.TypeText}}}
	q := &queueCore{replies: []string{`{"rows":[]}`, `{"rows":[{"Номер":"1"},{"Номер":"2"}]}`}}
	rows, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), tpl, "p.pdf", imgPages(2), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Cells["Номер"].Value != "" || rows[1].Cells["Номер"].Value != "1" || rows[1].Page != 2 {
		t.Fatalf("rows %+v", rows)
	}
}

func TestExtractParsesFencedJSONAndIgnoresExtraColumns(t *testing.T) {
	q := &queueCore{replies: []string{"Вот результат:\n```json\n{\"rows\":[{\"ФИО\":\"Абдыхалыкοву\",\"Лишняя\":{\"value\":\"x\"}}]}\n```\nГотово."}}
	rows, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), driverTpl, "a.pdf", imgPages(1), false)
	if err != nil {
		t.Fatal(err)
	}
	c := rows[0].Cells
	if c["ФИО"].Value != "Абдыхалыкову" || c["ФИО"].Confidence != "high" {
		t.Fatalf("ФИО %+v (греческая ο должна стать кириллической)", c["ФИО"])
	}
	if _, ok := c["Лишняя"]; ok || c["ИИН"].Value != "" {
		t.Fatalf("cells %+v", c)
	}
}

func TestExtractRetriesBadJSONOnce(t *testing.T) {
	q := &queueCore{replies: []string{"не json", `{"rows":[{"ФИО":"А"}]}`}}
	if _, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), driverTpl, "a.pdf", imgPages(1), false); err != nil {
		t.Fatal(err)
	}
	if len(q.reqs) != 2 || !strings.Contains(q.reqs[1].Text, "JSON") {
		t.Fatalf("повтор: %d запросов", len(q.reqs))
	}
	q = &queueCore{replies: []string{"не json", "опять не json"}}
	_, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), driverTpl, "a.pdf", imgPages(1), false)
	if !errors.Is(err, ErrBadJSON) || len(q.reqs) != 2 {
		t.Fatalf("err %v запросов %d", err, len(q.reqs))
	}
}

func TestExtractWithReviewMarksChangedCellsLow(t *testing.T) {
	q := &queueCore{replies: []string{
		`{"rows":[{"ФИО":{"value":"Абдыхалыков","confidence":"high"},"ИИН":{"value":"850315400124","confidence":"high"}}]}`,
		`{"rows":[{"ФИО":{"value":"Әбдіхалықов","confidence":"high"},"ИИН":{"value":"850315400124","confidence":"high"}}]}`,
	}}
	rows, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), driverTpl, "a.pdf", imgPages(1), true)
	if err != nil {
		t.Fatal(err)
	}
	c := rows[0].Cells
	if c["ФИО"].Value != "Әбдіхалықов" || c["ФИО"].Confidence != "low" || c["ИИН"].Confidence != "high" {
		t.Fatalf("cells %+v", c)
	}
	if len(q.reqs) != 2 || !strings.HasPrefix(q.reqs[1].System, reviewRowsPrompt) || !strings.Contains(q.reqs[1].Text, "Абдыхалыков") {
		t.Fatal("второй запрос должен проверять строки первого прохода")
	}
}

func TestExtractTextOnlyPage(t *testing.T) {
	q := &queueCore{replies: []string{`{"rows":[{"ФИО":"Нұрсұлтан"}]}`}}
	pages := []PageText{{Page: normalize.Page{Source: "a.docx", Index: 1, TextLayer: "ФИО: Нұрсұлтан"}, Text: "ФИО: Нұрсұлтан"}}
	if _, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), driverTpl, "a.docx", pages, false); err != nil {
		t.Fatal(err)
	}
	if len(q.reqs[0].Images) != 0 || !strings.Contains(q.reqs[0].Text, "ФИО: Нұрсұлтан") {
		t.Fatalf("req %+v", q.reqs[0])
	}
}

func TestSuggest(t *testing.T) {
	q := &queueCore{replies: []string{"```json\n" + `{"name":"Справки","mode":"document","columns":[{"name":"ФИО","type":"text","hint":"полностью","required":true},{"name":"Дата","type":"date"}]}` + "\n```"}}
	tpl, err := (&Extractor{LLM: q, Model: "m"}).Suggest(context.Background(), "хочу ФИО и дату", nil)
	if err != nil || tpl.Name != "Справки" || len(tpl.Columns) != 2 || tpl.Columns[1].Type != tables.TypeDate {
		t.Fatalf("tpl %+v err %v", tpl, err)
	}
	if q.reqs[0].System != suggestPrompt || !strings.Contains(q.reqs[0].Text, "хочу ФИО и дату") {
		t.Fatalf("req %+v", q.reqs[0])
	}
	q = &queueCore{replies: []string{`{"name":"","columns":[{"name":"X","type":"weird"}]}`}}
	if _, err := (&Extractor{LLM: q, Model: "m"}).Suggest(context.Background(), "x", &PageText{Page: imgPage, Text: "образец"}); err == nil {
		t.Fatal("некорректный шаблон от модели должен давать ошибку")
	}
	if len(q.reqs[0].Images) != 1 || !strings.Contains(q.reqs[0].Text, "образец") {
		t.Fatal("образец документа должен уходить в запрос")
	}
}

var sumTpl = tables.Template{Name: "С", Mode: tables.ModeDocument, Columns: []tables.Column{
	{Name: "ФИО", Type: tables.TypeText},
	{Name: "Сумма", Type: tables.TypeMoney},
	{Name: "Оплачено", Type: tables.TypeBool},
}}

func extractOnce(t *testing.T, tpl tables.Template, replies ...string) ([]tables.Row, *queueCore, error) {
	t.Helper()
	q := &queueCore{replies: replies}
	rows, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), tpl, "a.pdf", imgPages(1), false)
	return rows, q, err
}

func TestExtractAcceptsBareObjectAndArray(t *testing.T) {
	rows, _, err := extractOnce(t, sumTpl, `{"ФИО":"Нұр","Сумма":"100"}`)
	if err != nil || rows[0].Cells["ФИО"].Value != "Нұр" {
		t.Fatalf("объект без rows: %+v %v", rows, err)
	}
	rows, _, err = extractOnce(t, sumTpl, `[{"ФИО":"Асқар"}]`)
	if err != nil || rows[0].Cells["ФИО"].Value != "Асқар" {
		t.Fatalf("массив: %+v %v", rows, err)
	}
	_, q, err := extractOnce(t, sumTpl, `{"foo":1}`, `{"rows":[{"ФИО":"А"}]}`)
	if err != nil || len(q.reqs) != 2 {
		t.Fatalf("JSON без строк и колонок должен повторяться: %v %d", err, len(q.reqs))
	}
}

func TestExtractAcceptsNumbersAndBooleans(t *testing.T) {
	rows, _, err := extractOnce(t, sumTpl, `{"rows":[{"Сумма":1250000.5,"Оплачено":{"value":true,"confidence":"low"},"ФИО":null}]}`)
	if err != nil {
		t.Fatal(err)
	}
	c := rows[0].Cells
	if c["Сумма"].Value != "1250000.5" || c["Оплачено"].Value != "true" || c["Оплачено"].Confidence != "low" || c["ФИО"].Value != "" {
		t.Fatalf("cells %+v", c)
	}
}

func TestExtractFindsJSONAmongBracedProse(t *testing.T) {
	rows, _, err := extractOnce(t, sumTpl, "Вот ответ {примечание: сумма в тенге}:\n```json\n{\"rows\":[{\"ФИО\":\"Нұр\"}]}\n```")
	if err != nil || rows[0].Cells["ФИО"].Value != "Нұр" {
		t.Fatalf("rows %+v err %v", rows, err)
	}
	rows, _, err = extractOnce(t, sumTpl, "Примечание {x}. Результат: {\"rows\":[{\"ФИО\":\"Асқар\"}]} конец {y}")
	if err != nil || rows[0].Cells["ФИО"].Value != "Асқар" {
		t.Fatalf("без ограждения: %+v err %v", rows, err)
	}
}

func TestReviewKeepsValuesTheReviewerOmitted(t *testing.T) {
	first := `{"rows":[{"ФИО":{"value":"Нұрсұлтан","confidence":"high"},"Сумма":{"value":"100","confidence":"high"}}]}`
	for _, review := range []string{`{"rows":[]}`, `{"rows":[{"Сумма":{"value":"100","confidence":"high"}}]}`} {
		q := &queueCore{replies: []string{first, review}}
		rows, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), sumTpl, "a.pdf", imgPages(1), true)
		if err != nil {
			t.Fatal(err)
		}
		if c := rows[0].Cells["ФИО"]; c.Value != "Нұрсұлтан" || c.Confidence != "high" {
			t.Fatalf("проверка %s стёрла значение: %+v", review, c)
		}
	}
}

func TestReviewRowCountChangeKeepsOriginalRowsFlagged(t *testing.T) {
	tpl := tables.Template{Name: "Н", Mode: tables.ModeTableRows, Columns: []tables.Column{{Name: "Товар", Type: tables.TypeText}}}
	q := &queueCore{replies: []string{`{"rows":[{"Товар":"Ұн"},{"Товар":"Қант"}]}`, `{"rows":[{"Товар":"Ұн"}]}`}}
	rows, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), tpl, "n.pdf", imgPages(1), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[1].Cells["Товар"].Value != "Қант" || rows[1].Cells["Товар"].Confidence != "low" || rows[0].Cells["Товар"].Confidence != "low" {
		t.Fatalf("rows %+v", rows)
	}
}

func TestPromptsVersion(t *testing.T) {
	v := PromptsVersion()
	if len(v) != 16 || v != PromptsVersion() {
		t.Fatalf("версия промптов %q", v)
	}
}

func TestExtractWithoutImagesSendsOnlyText(t *testing.T) {
	q := &queueCore{replies: []string{`{"rows":[{"ФИО":"А"}]}`, `{"rows":[{"ФИО":"А"}]}`}}
	_, err := (&Extractor{LLM: q, Model: "m", TextOnly: true}).Extract(context.Background(), driverTpl, "a.pdf", imgPages(3), true)
	if err != nil {
		t.Fatal(err)
	}
	for i, r := range q.reqs {
		if len(r.Images) != 0 {
			t.Fatalf("запрос %d отправил %d картинок", i, len(r.Images))
		}
	}
}

func TestReviewRejectsEchoedInstruction(t *testing.T) {
	q := &queueCore{replies: []string{"Страница 2. Проверь транскрипцию по изображению."}}
	got, err := (&Transcriber{LLM: q, Model: "m"}).Review(context.Background(), imgPage, "мәтін")
	if err != nil || got != "мәтін" {
		t.Fatalf("эхо инструкции должно отбрасываться: %q %v", got, err)
	}
}

func TestReviewSameValueInOtherNotationIsNotFlagged(t *testing.T) {
	tpl := tables.Template{Name: "Д", Mode: tables.ModeDocument, Columns: []tables.Column{
		{Name: "Дата", Type: tables.TypeDate},
		{Name: "Категории", Type: tables.TypeChoice, Options: []string{"B", "C"}},
		{Name: "ФИО", Type: tables.TypeText},
	}}
	first := `{"rows":[{"Дата":"2019 жылғы 1 қыркүйек","Категории":"В, С","ФИО":"Абдыхалыков"}]}`
	review := `{"rows":[{"Дата":"01.09.2019","Категории":"B, C","ФИО":"Әбдіхалықов"}]}`
	q := &queueCore{replies: []string{first, review}}
	rows, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), tpl, "a.docx", imgPages(1), true)
	if err != nil {
		t.Fatal(err)
	}
	c := rows[0].Cells
	if c["Дата"].Confidence != "high" || c["Категории"].Confidence != "high" {
		t.Fatalf("та же дата/категории в другой записи не должны помечаться: %+v", c)
	}
	if c["ФИО"].Confidence != "low" || c["ФИО"].Value != "Әбдіхалықов" {
		t.Fatalf("настоящая правка должна помечаться: %+v", c["ФИО"])
	}
}

// Локальная модель иногда отвечает JSON своего устройства — такие «строки» без колонок шаблона
// не должны становиться пустыми строками таблицы: это ошибка формата, запрос повторяется.
func TestExtractRejectsRowsWithoutTemplateColumns(t *testing.T) {
	q := &queueCore{replies: []string{
		`{"rows":[{"номер":1,"заголовок":"ЖЕКЕ КУӘЛІК"},{"номер":2}]}`,
		`{"rows":[{"ФИО":"Тестов Иван"}]}`,
	}}
	rows, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), driverTpl, "a.jpg", imgPages(1), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(q.reqs) != 2 || len(rows) != 1 || rows[0].Cells["ФИО"].Value != "Тестов Иван" {
		t.Fatalf("запросов %d, строки %+v", len(q.reqs), rows)
	}
}

// Схема ответа передаётся модели: у Ollama она включает строгий JSON-режим.
func TestExtractSendsAnswerSchema(t *testing.T) {
	q := &queueCore{replies: []string{`{"rows":[{"ФИО":"А"}]}`}}
	if _, err := (&Extractor{LLM: q, Model: "m"}).Extract(context.Background(), driverTpl, "a.jpg", imgPages(1), false); err != nil {
		t.Fatal(err)
	}
	var schema struct {
		Properties struct {
			Rows struct {
				Items struct {
					Properties map[string]any `json:"properties"`
					Required   []string       `json:"required"`
				} `json:"items"`
			} `json:"rows"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(q.reqs[0].Schema, &schema); err != nil {
		t.Fatalf("схема %s: %v", q.reqs[0].Schema, err)
	}
	items := schema.Properties.Rows.Items
	if len(items.Properties) != len(driverTpl.Columns) || len(items.Required) != len(driverTpl.Columns) {
		t.Fatalf("схема %s", q.reqs[0].Schema)
	}
	if _, ok := items.Properties["ФИО"]; !ok {
		t.Fatalf("нет колонки ФИО в схеме %s", q.reqs[0].Schema)
	}
}

// Разметка образцов (отпечаток, образец карточки, зоны) — служебные данные программы: в описание
// шаблона для модели они не попадают, иначе десятки килобайт base64 вытесняют инструкции.
func TestExtractPromptOmitsFormsData(t *testing.T) {
	tpl := driverTpl
	tpl.Forms = []tables.Form{{Name: "Удостоверение", Signature: strings.Repeat("A", 3000), Locator: strings.Repeat("B", 20000),
		Zones: map[string]tables.Zone{"ФИО": {X: 0.1, Y: 0.1, W: 0.2, H: 0.1}}}}
	q := &queueCore{replies: []string{`{"rows":[{"ФИО":"А"}]}`}}
	if _, err := (&Extractor{LLM: q, Model: "m"}).extract(context.Background(), tpl, "a.jpg", imgPages(1), false); err != nil {
		t.Fatal(err)
	}
	sys := q.reqs[0].System
	if strings.Contains(sys, "AAAA") || strings.Contains(sys, "BBBB") || strings.Contains(sys, "Удостоверение\"") {
		t.Fatalf("в промпте разметка образцов (%d байт)", len(sys))
	}
	if !strings.Contains(sys, "ФИО") {
		t.Fatal("в промпте нет колонок")
	}
}
