package main

import (
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
)

type client struct {
	ID      int64
	Name    string
	Email   string
	Address string
}

type item struct {
	ID             int64
	Description    string
	Quantity       int
	UnitPriceCents int64
}

type invoice struct {
	ID         int64
	Number     string
	ClientID   int64
	ClientName string
	IssueDate  string
	DueDate    string
	Status     string
	Notes      string
	TotalCents int64
	Items      []item
}

func serverError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func parseCents(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("montant vide")
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("montant invalide: %q", s)
	}
	return int64(math.Round(v * 100)), nil
}

func formatCents(c int64) string {
	return fmt.Sprintf("%.2f", float64(c)/100)
}

func (s *server) allClients() ([]client, error) {
	rows, err := s.db.Query("SELECT id, name, email, address FROM clients ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []client
	for rows.Next() {
		var c client
		if err := rows.Scan(&c.ID, &c.Name, &c.Email, &c.Address); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *server) invoices(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
SELECT i.id, i.number, i.client_id, c.name, i.issue_date, i.due_date, i.status,
       COALESCE(SUM(ii.quantity * ii.unit_price_cents), 0)
FROM invoices i
JOIN clients c ON c.id = i.client_id
LEFT JOIN invoice_items ii ON ii.invoice_id = i.id
GROUP BY i.id
ORDER BY i.id DESC`)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()

	var list []invoice
	var total, paid int64
	for rows.Next() {
		var in invoice
		if err := rows.Scan(&in.ID, &in.Number, &in.ClientID, &in.ClientName,
			&in.IssueDate, &in.DueDate, &in.Status, &in.TotalCents); err != nil {
			serverError(w, err)
			return
		}
		total += in.TotalCents
		if in.Status == "paid" {
			paid += in.TotalCents
		}
		list = append(list, in)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}

	s.render(w, "index", map[string]any{
		"Title":       "Factures",
		"Invoices":    list,
		"TotalCents":  total,
		"PaidCents":   paid,
		"UnpaidCents": total - paid,
	})
}

func (s *server) newInvoice(w http.ResponseWriter, r *http.Request) {
	clients, err := s.allClients()
	if err != nil {
		serverError(w, err)
		return
	}
	s.render(w, "invoice_form", map[string]any{
		"Title": "Nouvelle facture", "Clients": clients, "Today": today(),
	})
}

func (s *server) createInvoice(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	clientID, err := strconv.ParseInt(r.FormValue("client_id"), 10, 64)
	if err != nil {
		http.Error(w, "Client invalide", http.StatusBadRequest)
		return
	}
	issueDate := r.FormValue("issue_date")
	if issueDate == "" {
		issueDate = today()
	}
	dueDate := r.FormValue("due_date")
	notes := r.FormValue("notes")

	descs, qtys, prices := r.Form["desc"], r.Form["qty"], r.Form["price"]
	type parsedItem struct {
		desc  string
		qty   int
		cents int64
	}
	var items []parsedItem
	for i, d := range descs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		qty := 1
		if i < len(qtys) {
			if n, err := strconv.Atoi(strings.TrimSpace(qtys[i])); err == nil && n > 0 {
				qty = n
			}
		}
		cents := int64(0)
		if i < len(prices) {
			cents, err = parseCents(prices[i])
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		items = append(items, parsedItem{d, qty, cents})
	}
	if len(items) == 0 {
		clients, _ := s.allClients()
		s.render(w, "invoice_form", map[string]any{
			"Title": "Nouvelle facture", "Clients": clients, "Today": today(),
			"Error": "Ajoutez au moins une ligne avec une description.",
		})
		return
	}

	tx, err := s.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()

	number, err := nextInvoiceNumber(tx)
	if err != nil {
		serverError(w, err)
		return
	}
	res, err := tx.Exec(`INSERT INTO invoices
(number, client_id, issue_date, due_date, status, notes, created_at)
VALUES (?, ?, ?, ?, 'unpaid', ?, ?)`, number, clientID, issueDate, dueDate, notes, nowRFC3339())
	if err != nil {
		serverError(w, err)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		serverError(w, err)
		return
	}
	for _, it := range items {
		if _, err := tx.Exec(`INSERT INTO invoice_items
(invoice_id, description, quantity, unit_price_cents) VALUES (?, ?, ?, ?)`,
			id, it.desc, it.qty, it.cents); err != nil {
			serverError(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/invoices/%d", id), http.StatusSeeOther)
}

func (s *server) invoiceDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	var in invoice
	err = s.db.QueryRow(`
SELECT i.id, i.number, i.client_id, c.name, i.issue_date, i.due_date, i.status, i.notes,
       COALESCE(SUM(ii.quantity * ii.unit_price_cents), 0)
FROM invoices i JOIN clients c ON c.id = i.client_id
LEFT JOIN invoice_items ii ON ii.invoice_id = i.id
WHERE i.id = ? GROUP BY i.id`, id).Scan(&in.ID, &in.Number, &in.ClientID, &in.ClientName,
		&in.IssueDate, &in.DueDate, &in.Status, &in.Notes, &in.TotalCents)
	if err == sql.ErrNoRows {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, err)
		return
	}
	rows, err := s.db.Query(`SELECT id, description, quantity, unit_price_cents
FROM invoice_items WHERE invoice_id = ? ORDER BY id`, id)
	if err != nil {
		serverError(w, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ID, &it.Description, &it.Quantity, &it.UnitPriceCents); err != nil {
			serverError(w, err)
			return
		}
		in.Items = append(in.Items, it)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}
	s.render(w, "invoice_detail", map[string]any{
		"Title": in.Number, "Invoice": in,
	})
}

func (s *server) payInvoice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, err := s.db.Exec("UPDATE invoices SET status = 'paid' WHERE id = ?", id); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/invoices/%d", id), http.StatusSeeOther)
}

func (s *server) deleteInvoice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		serverError(w, err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM invoice_items WHERE invoice_id = ?", id); err != nil {
		serverError(w, err)
		return
	}
	if _, err := tx.Exec("DELETE FROM invoices WHERE id = ?", id); err != nil {
		serverError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) clients(w http.ResponseWriter, r *http.Request) {
	list, err := s.allClients()
	if err != nil {
		serverError(w, err)
		return
	}
	s.render(w, "clients", map[string]any{"Title": "Clients", "Clients": list})
}

func (s *server) createClient(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		list, _ := s.allClients()
		s.render(w, "clients", map[string]any{
			"Title": "Clients", "Clients": list, "Error": "Le nom est obligatoire.",
		})
		return
	}
	_, err := s.db.Exec(`INSERT INTO clients (name, email, address, created_at)
VALUES (?, ?, ?, ?)`, name, r.FormValue("email"), r.FormValue("address"), nowRFC3339())
	if err != nil {
		serverError(w, err)
		return
	}
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}
