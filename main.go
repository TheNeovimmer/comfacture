package main

import (
	"database/sql"
	"embed"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
)

const currency = "€"

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/*
var staticFS embed.FS

type server struct {
	db        *sql.DB
	templates map[string]*template.Template
}

func loadTemplates() (map[string]*template.Template, error) {
	funcs := template.FuncMap{
		"money": func(c int64) string { return currency + " " + formatCents(c) },
		"mul":   func(cents int64, qty int) int64 { return cents * int64(qty) },
		"today": today,
	}
	out := map[string]*template.Template{}
	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".html")
		if name == "base" {
			continue
		}
		t, err := template.New("layout").Funcs(funcs).ParseFS(templateFS,
			"templates/base.html", "templates/"+e.Name())
		if err != nil {
			return nil, err
		}
		out[name] = t
	}
	return out, nil
}

func (s *server) render(w http.ResponseWriter, name string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["Title"]; !ok {
		data["Title"] = name
	}
	if _, ok := data["Section"]; !ok {
		switch name {
		case "clients":
			data["Section"] = "clients"
		case "import":
			data["Section"] = "import"
		case "stats":
			data["Section"] = "stats"
		default:
			data["Section"] = "factures"
		}
	}
	t, ok := s.templates[name]
	if !ok {
		http.Error(w, "Template introuvable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.invoices)
	mux.HandleFunc("GET /invoices/new", s.newInvoice)
	mux.HandleFunc("POST /invoices", s.createInvoice)
	mux.HandleFunc("GET /invoices/{id}", s.invoiceDetail)
	mux.HandleFunc("GET /invoices/export.xlsx", s.exportXLSX)
	mux.HandleFunc("POST /invoices/{id}/pay", s.payInvoice)
	mux.HandleFunc("POST /invoices/{id}/delete", s.deleteInvoice)
	mux.HandleFunc("GET /stats", s.stats)
	mux.HandleFunc("GET /clients", s.clients)
	mux.HandleFunc("POST /clients", s.createClient)
	mux.HandleFunc("GET /import", s.importPage)
	mux.HandleFunc("POST /import", s.importXLSX)
	mux.HandleFunc("POST /api/chat", s.chat)

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))
	return mux
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	db, err := openDB(env("DB_PATH", "invoices.db"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	tmpl, err := loadTemplates()
	if err != nil {
		log.Fatal(err)
	}
	s := &server{db: db, templates: tmpl}

	addr := ":" + env("PORT", "8080")
	log.Printf("Gestion des factures disponible sur http://localhost%s", addr)
	log.Fatal(http.ListenAndServe(addr, s.routes()))
}
