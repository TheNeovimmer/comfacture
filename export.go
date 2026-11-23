package main

import (
	"fmt"
	"net/http"

	"github.com/xuri/excelize/v2"
)

// exportXLSX writes one row per invoice: number, client, dates, status and total.
func (s *server) exportXLSX(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`
SELECT i.number, c.name, i.issue_date, i.due_date, i.status,
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

	type exportRow struct {
		number, client, issued, due, status string
		totalCents                          int64
	}
	var data []exportRow
	var grandTotal int64
	for rows.Next() {
		var e exportRow
		if err := rows.Scan(&e.number, &e.client, &e.issued, &e.due, &e.status, &e.totalCents); err != nil {
			serverError(w, err)
			return
		}
		grandTotal += e.totalCents
		data = append(data, e)
	}
	if err := rows.Err(); err != nil {
		serverError(w, err)
		return
	}

	f := excelize.NewFile()
	defer f.Close()
	const sheet = "Factures"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		serverError(w, err)
		return
	}
	header := []any{"Numéro", "Client", "Émission", "Échéance", "Statut", "Total (€)"}
	if err := f.SetSheetRow(sheet, "A1", &header); err != nil {
		serverError(w, err)
		return
	}
	for i, e := range data {
		status := "Impayée"
		if e.status == "paid" {
			status = "Payée"
		}
		row := []any{e.number, e.client, e.issued, e.due, status, float64(e.totalCents) / 100}
		if err := f.SetSheetRow(sheet, fmt.Sprintf("A%d", i+2), &row); err != nil {
			serverError(w, err)
			return
		}
	}
	last := len(data) + 2
	totalRow := []any{"", "", "", "", "Total", float64(grandTotal) / 100}
	if err := f.SetSheetRow(sheet, fmt.Sprintf("A%d", last), &totalRow); err != nil {
		serverError(w, err)
		return
	}

	bold, err := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	if err != nil {
		serverError(w, err)
		return
	}
	if err := f.SetCellStyle(sheet, "A1", "F1", bold); err != nil {
		serverError(w, err)
		return
	}
	if err := f.SetCellStyle(sheet, fmt.Sprintf("A%d", last), fmt.Sprintf("F%d", last), bold); err != nil {
		serverError(w, err)
		return
	}
	f.SetColWidth(sheet, "A", "F", 18)
	f.SetColWidth(sheet, "B", "B", 28)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="factures.xlsx"`)
	if err := f.Write(w); err != nil {
		serverError(w, err)
	}
}
