package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// openDB opens the SQLite database, enables the pragmas we rely on and creates
// missing tables. SetMaxOpenConns(1) keeps pragma settings on a single connection.
func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{"PRAGMA foreign_keys=ON", "PRAGMA journal_mode=WAL"} {
		if _, err := db.Exec(pragma); err != nil {
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	if err := initSchema(db); err != nil {
		return nil, err
	}
	return db, nil
}

func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS clients (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    name       TEXT NOT NULL,
    email      TEXT NOT NULL DEFAULT '',
    address    TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS invoices (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    number     TEXT NOT NULL UNIQUE,
    client_id  INTEGER NOT NULL REFERENCES clients(id),
    issue_date TEXT NOT NULL,
    due_date   TEXT NOT NULL DEFAULT '',
    status     TEXT NOT NULL DEFAULT 'unpaid',
    notes      TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS invoice_items (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    invoice_id      INTEGER NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    description     TEXT NOT NULL,
    quantity        INTEGER NOT NULL DEFAULT 1,
    unit_price_cents INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_invoices_client ON invoices(client_id);
CREATE INDEX IF NOT EXISTS idx_items_invoice ON invoice_items(invoice_id);

-- One semantic layer for every aggregate: a fact row per invoice with its total.
CREATE VIEW IF NOT EXISTS invoice_facts AS
SELECT i.id, i.number, i.client_id, c.name AS client_name,
       i.issue_date, i.due_date, i.status,
       COALESCE(SUM(ii.quantity * ii.unit_price_cents), 0) AS total_cents
FROM invoices i
JOIN clients c ON c.id = i.client_id
LEFT JOIN invoice_items ii ON ii.invoice_id = i.id
GROUP BY i.id;
`)
	return err
}

func nextInvoiceNumber(tx *sql.Tx) (string, error) {
	var n int
	if err := tx.QueryRow("SELECT COUNT(*) FROM invoices").Scan(&n); err != nil {
		return "", err
	}
	for {
		n++
		number := fmt.Sprintf("F-%s-%04d", today()[:4], n)
		var exists int
		if err := tx.QueryRow("SELECT COUNT(*) FROM invoices WHERE number = ?", number).Scan(&exists); err != nil {
			return "", err
		}
		if exists == 0 {
			return number, nil
		}
	}
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

func today() string { return time.Now().Format("2006-01-02") }
