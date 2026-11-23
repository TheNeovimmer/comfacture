package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

var defaultModels = []string{
	"google/gemma-4-26b-a4b-it:free",
	"google/gemma-4-31b-it:free",
	"nvidia/nemotron-3.5-lightning:free",
}

type llmResult struct {
	Clients  []llmClient  `json:"clients"`
	Invoices []llmInvoice `json:"invoices"`
}

type llmClient struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Address string `json:"address"`
}

type llmInvoice struct {
	Client    string    `json:"client"`
	Number    string    `json:"number"`
	IssueDate string    `json:"issue_date"`
	DueDate   string    `json:"due_date"`
	Status    string    `json:"status"`
	Notes     string    `json:"notes"`
	Items     []llmItem `json:"items"`
}

type llmItem struct {
	Description string  `json:"description"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
}

func extractXLSXRrows(data []byte) ([][]string, error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("aucune feuille")
	}
	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("feuille vide")
	}
	return rows, nil
}

func rowsToText(rows [][]string) string {
	var b strings.Builder
	for i, row := range rows {
		if i >= 100 {
			break
		}
		cells := make([]string, 0, len(row))
		for _, c := range row {
			c = strings.ReplaceAll(strings.ReplaceAll(c, "\t", " "), "\n", " ")
			if len(c) > 120 {
				c = c[:120]
			}
			cells = append(cells, c)
		}
		b.WriteString(strings.Join(cells, "\t"))
		b.WriteByte('\n')
	}
	return b.String()
}

func cleanJSON(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func candidateModels() []string {
	models := []string{}
	if m := strings.TrimSpace(os.Getenv("OPENROUTER_MODEL")); m != "" {
		models = append(models, m)
	}
	return append(models, defaultModels...)
}

func askModel(key, model string, messages []map[string]string) (string, error) {
	body := map[string]any{
		"model":       model,
		"messages":    messages,
		"temperature": 0,
		"max_tokens":  2000,
		"reasoning":   map[string]bool{"enabled": false},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.Error != nil {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("aucune réponse du modèle")
	}
	content := strings.TrimSpace(out.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("réponse vide")
	}
	return content, nil
}

// callOpenRouter tries the configured model first, then free fallbacks.
func callOpenRouter(rows string) ([]byte, error) {
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return nil, fmt.Errorf("OPENROUTER_API_KEY non définie")
	}
	models := candidateModels()

	prompt := `Convertis ces lignes de tableur (séparées par des tabulations, première ligne = en-tête) en factures.

` + rows + `
Réponds avec UNIQUEMENT un objet JSON de cette forme :
{"clients":[{"name":"","email":"","address":""}],"invoices":[{"client":"","number":"","issue_date":"","due_date":"","status":"","notes":"","items":[{"description":"","quantity":1,"unit_price":0}]}]}

Règles :
- "client" reprend le nom d'un client de "clients", ou un nouveau nom (il sera créé).
- Dates au format AAAA-MM-JJ, chaîne vide si absente.
- status vaut "paid" ou "unpaid".
- quantity entier >= 1, unit_price nombre sans symbole monétaire (ex: 12.50).
- Si une ligne n'a que montant total et aucune description, crée un article "Import" avec ce montant.
- S'il n'y a aucune donnée, réponds {"clients":[],"invoices":[]}.`

	var lastErr error
	seen := map[string]bool{}
	for _, model := range models {
		if seen[model] {
			continue
		}
		seen[model] = true
		messages := []map[string]string{
			{"role": "system", "content": "Tu convertis des données en JSON. Réponds uniquement avec un objet JSON valide, sans markdown ni explication."},
			{"role": "user", "content": prompt},
		}
		content, err := askModel(key, model, messages)
		if err != nil {
			lastErr = fmt.Errorf("%s: %w", model, err)
			continue
		}
		cleaned := cleanJSON(content)
		if !json.Valid([]byte(cleaned)) {
			lastErr = fmt.Errorf("%s: réponse non JSON (%.80s)", model, cleaned)
			continue
		}
		return []byte(cleaned), nil
	}
	return nil, lastErr
}

func (s *server) storeImport(result llmResult) (int, int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()

	ids := map[string]int64{}
	rows, err := tx.Query("SELECT id, name FROM clients")
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return 0, 0, err
		}
		ids[name] = id
	}
	rows.Close()

	ensureClient := func(name, email, address string) (int64, error) {
		name = strings.TrimSpace(name)
		if name == "" {
			return 0, fmt.Errorf("client sans nom")
		}
		if id, ok := ids[name]; ok {
			return id, nil
		}
		res, err := tx.Exec(`INSERT INTO clients (name, email, address, created_at) VALUES (?, ?, ?, ?)`,
			name, email, address, nowRFC3339())
		if err != nil {
			return 0, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, err
		}
		ids[name] = id
		return id, nil
	}

	for _, c := range result.Clients {
		if _, err := ensureClient(c.Name, c.Email, c.Address); err != nil {
			return 0, 0, err
		}
	}
	for _, in := range result.Invoices {
		clientID, err := ensureClient(in.Client, "", "")
		if err != nil {
			return 0, 0, err
		}
		number := strings.TrimSpace(in.Number)
		exists := 0
		if number != "" {
			if err := tx.QueryRow("SELECT COUNT(*) FROM invoices WHERE number = ?", number).Scan(&exists); err != nil {
				return 0, 0, err
			}
		}
		if number == "" || exists > 0 {
			number, err = nextInvoiceNumber(tx)
			if err != nil {
				return 0, 0, err
			}
		}
		status := in.Status
		if status != "paid" {
			status = "unpaid"
		}
		issue := in.IssueDate
		if issue == "" {
			issue = today()
		}
		res, err := tx.Exec(`INSERT INTO invoices
(number, client_id, issue_date, due_date, status, notes, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`, number, clientID, issue, in.DueDate, status, in.Notes, nowRFC3339())
		if err != nil {
			return 0, 0, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return 0, 0, err
		}
		for _, it := range in.Items {
			desc := strings.TrimSpace(it.Description)
			if desc == "" {
				desc = "Import"
			}
			qty := it.Quantity
			if qty < 1 {
				qty = 1
			}
			cents := int64(math.Round(it.UnitPrice * 100))
			if _, err := tx.Exec(`INSERT INTO invoice_items
(invoice_id, description, quantity, unit_price_cents) VALUES (?, ?, ?, ?)`,
				id, desc, qty, cents); err != nil {
				return 0, 0, err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return len(result.Clients), len(result.Invoices), nil
}

func (s *server) importPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, "import", map[string]any{"Title": "Importer XLSX"})
}

func (s *server) importXLSX(w http.ResponseWriter, r *http.Request) {
	showError := func(msg string) {
		s.render(w, "import", map[string]any{"Title": "Importer XLSX", "Error": msg})
	}
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		showError("Fichier illisible ou trop volumineux (10 Mo max).")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		showError("Choisissez un fichier .xlsx.")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		showError("Lecture du fichier impossible.")
		return
	}
	rows, err := extractXLSXRrows(data)
	if err != nil {
		showError("Fichier Excel illisible : " + err.Error())
		return
	}
	raw, err := callOpenRouter(rowsToText(rows))
	if err != nil {
		showError("Erreur OpenRouter : " + err.Error())
		return
	}
	var result llmResult
	if err := json.Unmarshal(raw, &result); err != nil {
		showError("Réponse du modèle invalide : " + err.Error())
		return
	}
	clients, invoices, err := s.storeImport(result)
	if err != nil {
		showError("Import en base impossible : " + err.Error())
		return
	}
	s.render(w, "import", map[string]any{
		"Title":  "Importer XLSX",
		"Result": fmt.Sprintf("%d client(s) et %d facture(s) importés.", clients, invoices),
	})
}
