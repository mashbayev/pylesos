// Файл: internal/texts/ru_test.go
package texts

import (
	"testing"

	"pylesos/internal/validate"
)

// Каждое замечание проверки значений имеет русский текст.
func TestEveryCheckIssueHasText(t *testing.T) {
	for _, key := range validate.IssueKeys() {
		if CheckIssue(key) == key {
			t.Fatalf("нет текста для %s", key)
		}
	}
}
