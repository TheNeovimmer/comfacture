package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// invoiceContext is a compact, read-only snapshot given to the model.
func (s *server) invoiceContext() (string, error) {
	rows, err := s.db.Query(`
SELECT i.number, c.name, i.issue_date, i.due_date, i.status,
       COALESCE(SUM(ii.quantity * ii.unit_price_cents), 0)
FROM invoices i
JOIN clients c ON c.id = i.client_id
LEFT JOIN invoice_items ii ON ii.invoice_id = i.id
GROUP BY i.id
ORDER BY i.due_date, i.id`)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var total, paid, unpaidCount, paidCount int64
	var lines []string
	for rows.Next() {
		var number, client, issued, due, status string
		var cents int64
		if err := rows.Scan(&number, &client, &issued, &due, &status, &cents); err != nil {
			return "", err
		}
		total += cents
		if status == "paid" {
			paid += cents
			paidCount++
		} else {
			unpaidCount++
			lines = append(lines, fmt.Sprintf("- %s | %s | échéance %s | %.2f € | impayée",
				number, client, due, float64(cents)/100))
		}
		if len(lines) >= 30 {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	var clients int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM clients").Scan(&clients); err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d client(s), %d facture(s) dont %d impayée(s).\n", clients, unpaidCount+paidCount, unpaidCount)
	fmt.Fprintf(&b, "Total facturé %.2f €, encaissé %.2f €, restant dû %.2f €.\n",
		float64(total)/100, float64(paid)/100, float64(total-paid)/100)
	if len(lines) == 0 {
		b.WriteString("Aucune facture impayée.\n")
	} else {
		b.WriteString("Factures impayées (échéance la plus proche en premier) :\n")
		b.WriteString(strings.Join(lines, "\n"))
	}
	return b.String(), nil
}

func chatReply(ctx, message string) (string, error) {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return "", fmt.Errorf("OPENROUTER_API_KEY non définie")
	}
	messages := []map[string]string{
		{"role": "system", "content": "Tu es l'assistant d'un logiciel de facturation pour PME. Réponds en français, en trois phrases maximum, uniquement à partir des données fournies. Ne prétends jamais avoir modifié une donnée. Si l'information manque, dis-le simplement."},
		{"role": "user", "content": "Données des factures :\n" + ctx + "\n\nQuestion : " + message},
	}
	var lastErr error
	seen := map[string]bool{}
	for _, model := range candidateModels() {
		if seen[model] {
			continue
		}
		seen[model] = true
		content, err := askModel(key, model, messages)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", model, err)
			continue
		}
		return content, nil
	}
	return "", lastErr
}

func (s *server) chat(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		http.Error(w, "Requête invalide", http.StatusBadRequest)
		return
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		http.Error(w, "Message vide", http.StatusBadRequest)
		return
	}
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	ctx, err := s.invoiceContext()
	if err != nil {
		serverError(w, err)
		return
	}
	reply, err := chatReply(ctx, msg)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"reply": reply})
}
