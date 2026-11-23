package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func makeXLSX(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	if err := f.SetSheetRow("Sheet1", "A1", &[]any{"Numéro", "Client", "Total"}); err != nil {
		t.Fatal(err)
	}
	if err := f.SetSheetRow("Sheet1", "A2", &[]any{"F-001", "ACME", 25.5}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractXLSXRrows(t *testing.T) {
	rows, err := extractXLSXRrows(makeXLSX(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0][1] != "Client" || rows[1][1] != "ACME" {
		t.Fatalf("unexpected rows: %v", rows)
	}
	if text := rowsToText(rows); !strings.Contains(text, "F-001\tACME\t25.5") {
		t.Errorf("unexpected text: %q", text)
	}
}

func TestCleanJSON(t *testing.T) {
	in := "```json\n{\"clients\":[]}\n```"
	if got := cleanJSON(in); got != "{\"clients\":[]}" {
		t.Errorf("cleanJSON = %q", got)
	}
}

func TestStoreImport(t *testing.T) {
	s := newTestServer(t)
	clients, invoices, err := s.storeImport(llmResult{
		Clients: []llmClient{{Name: "ACME", Email: "a@b.com"}},
		Invoices: []llmInvoice{{
			Client: "ACME", IssueDate: "2026-11-23", Status: "paid",
			Items: []llmItem{{Description: "Dev", Quantity: 3, UnitPrice: 12.5}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if clients != 1 || invoices != 1 {
		t.Fatalf("counts = %d clients, %d invoices", clients, invoices)
	}
	var total int64
	if err := s.db.QueryRow(`SELECT COALESCE(SUM(quantity * unit_price_cents),0)
FROM invoice_items`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 3750 {
		t.Errorf("total cents = %d, want 3750", total)
	}
}

func TestImportWithoutAPIKey(t *testing.T) {
	t.Setenv("OPENROUTER_API_KEY", "")
	s := newTestServer(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "test.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(makeXLSX(t)); err != nil {
		t.Fatal(err)
	}
	mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/import", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "OPENROUTER_API_KEY non définie") {
		t.Errorf("missing key error not shown: %s", w.Body.String())
	}
}
