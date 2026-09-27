// Файл: internal/table/domain/domain.go
// Package domain — таблица Excel по людям: колонки из конструктора (П29) и настройка выгрузки (П20).
package domain

import "pylesos/internal/tables"

type Config struct {
	Columns        []tables.Column `json:"columns"`
	IncludeQueries bool            `json:"include_queries"`
}

// Person — человек для выгрузки: известные поля из разбора.
type Person struct {
	ID        int64  `db:"id"`
	FullName  string `db:"full_name"`
	IIN       string `db:"iin"`
	DocNumber string `db:"doc_number"`
	BirthDate string `db:"birth_date"`
}
