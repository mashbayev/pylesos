// Файл: internal/api/forms_test.go
package api

import (
	"io"
	"net/http"
	"testing"
)

func TestFormsRoutes(t *testing.T) {
	_, ts, fs, _ := start(t, fakeJobs{})
	if resp := do(t, http.MethodGet, ts.URL+"/api/screens/forms?id=7", "секрет", ""); resp.StatusCode != http.StatusOK || fs.saved["open"] != "7" {
		t.Fatalf("экран форм: %d %v", resp.StatusCode, fs.saved)
	}
	do(t, http.MethodPost, ts.URL+"/api/forms", "секрет", `{"name":"Права","file_name":"a.png","data_base64":"0L/RgNC40LLQtdGC"}`)
	if fs.saved["name"] != "Права" || fs.saved["file"] != "a.png" || fs.saved["data"] != "привет" {
		t.Fatalf("создание: %v", fs.saved)
	}
	if resp := do(t, http.MethodPost, ts.URL+"/api/forms", "секрет", `{"name":"x","data_base64":"не base64"}`); resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("битый base64: %d", resp.StatusCode)
	}
	do(t, http.MethodPost, ts.URL+"/api/forms/3/zone", "секрет", `{"key":"iin","x":0.25,"y":0.1,"w":0.2,"h":0.1}`)
	if fs.saved["id"] != "3" || fs.saved["key"] != "iin" || fs.saved["x"] != "0.25" {
		t.Fatalf("зона: %v", fs.saved)
	}
	do(t, http.MethodPost, ts.URL+"/api/forms/3/clear", "секрет", `{"key":"iin"}`)
	if fs.saved["clear"] != "iin" {
		t.Fatalf("снять: %v", fs.saved)
	}
	do(t, http.MethodPost, ts.URL+"/api/forms/3/rename", "секрет", `{"name":"Права (новые)"}`)
	if fs.saved["rename"] != "Права (новые)" {
		t.Fatalf("переименовать: %v", fs.saved)
	}
	do(t, http.MethodPost, ts.URL+"/api/forms/3/delete", "секрет", `{}`)
	if fs.saved["delete"] != "3" {
		t.Fatalf("удалить: %v", fs.saved)
	}
}

func TestFormSample(t *testing.T) {
	_, ts, _, _ := start(t, fakeJobs{})
	resp := do(t, http.MethodGet, ts.URL+"/api/forms/5/sample?t=секрет", "", "")
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" || string(body) != "jpeg-5" {
		t.Fatalf("образец: %d %q %q %v", resp.StatusCode, resp.Header.Get("Content-Type"), body, err)
	}
	if resp := do(t, http.MethodGet, ts.URL+"/api/forms/404/sample?t=секрет", "", ""); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("нет формы: %d", resp.StatusCode)
	}
}
