package main

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestParseCents(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"12.50", 1250, true},
		{"12", 1200, true},
		{" 3.7 ", 370, true},
		{"", 0, false},
		{"abc", 0, false},
		{"-1", 0, false},
	}
	for _, c := range cases {
		got, err := parseCents(c.in)
		if (err == nil) != c.ok {
			t.Errorf("parseCents(%q): error=%v, want ok=%v", c.in, err, c.ok)
		}
		if got != c.want {
			t.Errorf("parseCents(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func newTestServer(t *testing.T) *server {
	t.Helper()
	db, err := openDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tmpl, err := loadTemplates()
	if err != nil {
		t.Fatal(err)
	}
	return &server{db: db, templates: tmpl}
}

func postForm(s *server, path string, values url.Values) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	return w
}

func TestInvoiceFlow(t *testing.T) {
	s := newTestServer(t)

	if w := postForm(s, "/clients", url.Values{"name": {"ACME"}}); w.Code != http.StatusSeeOther {
		t.Fatalf("create client: %d %s", w.Code, w.Body.String())
	}

	w := postForm(s, "/invoices", url.Values{
		"client_id":  {"1"},
		"issue_date": {"2026-11-23"},
		"desc":       {"Dev", "Design"},
		"qty":        {"3", "1"},
		"price":      {"12.50", "10"},
	})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("create invoice: %d %s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")

	r := httptest.NewRequest(http.MethodGet, loc, nil)
	w = httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("invoice detail: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "47.50") {
		t.Error("invoice detail is missing the computed total 47.50")
	}
	if !strings.Contains(w.Body.String(), "Due") {
		t.Error("invoice detail is missing the unpaid status")
	}

	if w := postForm(s, "/invoices/1/pay", url.Values{}); w.Code != http.StatusSeeOther {
		t.Fatalf("mark paid: %d", w.Code)
	}
	r = httptest.NewRequest(http.MethodGet, "/", nil)
	w = httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if !strings.Contains(w.Body.String(), "Payée") {
		t.Error("invoice list is missing the paid status")
	}
}

func TestExportXLSX(t *testing.T) {
	s := newTestServer(t)
	postForm(s, "/clients", url.Values{"name": {"ACME"}})
	postForm(s, "/invoices", url.Values{
		"client_id": {"1"}, "issue_date": {"2026-11-23"},
		"desc": {"Dev"}, "qty": {"2"}, "price": {"12.50"},
	})

	r := httptest.NewRequest(http.MethodGet, "/invoices/export.xlsx", nil)
	w := httptest.NewRecorder()
	s.routes().ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("export: %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "spreadsheetml") {
		t.Errorf("unexpected content type %q", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("not a valid xlsx zip: %v", err)
	}
	found := false
	for _, zf := range zr.File {
		if zf.Name == "xl/workbook.xml" {
			rc, err := zf.Open()
			if err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			found = strings.Contains(string(b), "Factures")
		}
	}
	if !found {
		t.Error("export is missing the Factures sheet")
	}
}
