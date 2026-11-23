package main

import (
	"net/http"
	"time"
)

type monthPoint struct {
	Label       string
	AmountCents int64
	Pct         float64
}

type clientPoint struct {
	Name       string
	Count      int
	TotalCents int64
	Pct        float64
}

type agingPoint struct {
	Label      string
	Count      int
	TotalCents int64
}

type statsData struct {
	Count, ClientCount, PaidCount, UnpaidCount, OverdueCount int
	TotalCents, PaidCents, UnpaidCents, OverdueCents         int64
	AverageCents                                             int64
	Months                                                   []monthPoint
	Clients                                                  []clientPoint
	Aging                                                    []agingPoint
}

var monthNames = []string{"janv.", "févr.", "mars", "avr.", "mai", "juin", "juil.", "août", "sept.", "oct.", "nov.", "déc."}

func (s *server) stats(w http.ResponseWriter, r *http.Request) {
	var d statsData
	todayStr := today()
	err := s.db.QueryRow(`
SELECT COUNT(*), COALESCE(SUM(total_cents), 0),
       COALESCE(SUM(CASE WHEN status = 'paid' THEN total_cents ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN status = 'unpaid' THEN total_cents ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN status = 'paid' THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN status = 'unpaid' AND due_date <> '' AND due_date < ? THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN status = 'unpaid' AND due_date <> '' AND due_date < ? THEN total_cents ELSE 0 END), 0)
FROM invoice_facts`, todayStr, todayStr).Scan(&d.Count, &d.TotalCents, &d.PaidCents, &d.UnpaidCents,
		&d.PaidCount, &d.OverdueCount, &d.OverdueCents)
	if err != nil {
		serverError(w, err)
		return
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM clients").Scan(&d.ClientCount); err != nil {
		serverError(w, err)
		return
	}
	d.UnpaidCount = d.Count - d.PaidCount
	if d.Count > 0 {
		d.AverageCents = d.TotalCents / int64(d.Count)
	}

	byMonth := map[string]int64{}
	rows, err := s.db.Query(`SELECT strftime('%Y-%m', issue_date), SUM(total_cents)
FROM invoice_facts WHERE issue_date <> '' GROUP BY 1 ORDER BY 1 DESC LIMIT 12`)
	if err != nil {
		serverError(w, err)
		return
	}
	for rows.Next() {
		var key string
		var cents int64
		if err := rows.Scan(&key, &cents); err != nil {
			rows.Close()
			serverError(w, err)
			return
		}
		byMonth[key] = cents
	}
	rows.Close()

	now := time.Now()
	var maxMonth int64 = 1
	for i := 11; i >= 0; i-- {
		t := time.Date(now.Year(), now.Month()-time.Month(i), 1, 0, 0, 0, 0, time.UTC)
		key := t.Format("2006-01")
		cents := byMonth[key]
		if cents > maxMonth {
			maxMonth = cents
		}
		d.Months = append(d.Months, monthPoint{
			Label:       monthNames[t.Month()-1] + " " + t.Format("2006"),
			AmountCents: cents,
		})
	}
	for i := range d.Months {
		d.Months[i].Pct = float64(d.Months[i].AmountCents) / float64(maxMonth) * 100
	}

	rows, err = s.db.Query(`SELECT client_name, COUNT(*), SUM(total_cents)
FROM invoice_facts GROUP BY client_id ORDER BY SUM(total_cents) DESC LIMIT 5`)
	if err != nil {
		serverError(w, err)
		return
	}
	var maxClient int64 = 1
	for rows.Next() {
		var p clientPoint
		if err := rows.Scan(&p.Name, &p.Count, &p.TotalCents); err != nil {
			rows.Close()
			serverError(w, err)
			return
		}
		if p.TotalCents > maxClient {
			maxClient = p.TotalCents
		}
		d.Clients = append(d.Clients, p)
	}
	rows.Close()
	for i := range d.Clients {
		d.Clients[i].Pct = float64(d.Clients[i].TotalCents) / float64(maxClient) * 100
	}

	d.Aging = []agingPoint{
		{Label: "Échue"},
		{Label: "0 – 30 jours"},
		{Label: "31 – 60 jours"},
		{Label: "Plus de 60 jours"},
		{Label: "Sans échéance"},
	}
	rows, err = s.db.Query("SELECT total_cents, due_date FROM invoice_facts WHERE status = 'unpaid'")
	if err != nil {
		serverError(w, err)
		return
	}
	for rows.Next() {
		var cents int64
		var due string
		if err := rows.Scan(&cents, &due); err != nil {
			rows.Close()
			serverError(w, err)
			return
		}
		idx := 4
		if due != "" {
			if t, err := time.Parse("2006-01-02", due); err == nil {
				days := int(time.Until(t).Hours() / 24)
				switch {
				case days < 0:
					idx = 0
				case days <= 30:
					idx = 1
				case days <= 60:
					idx = 2
				default:
					idx = 3
				}
			}
		}
		d.Aging[idx].Count++
		d.Aging[idx].TotalCents += cents
	}
	rows.Close()

	s.render(w, "stats", map[string]any{"Title": "Statistiques", "Stats": d})
}
