package main

import (
	"context"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed templates/*
var templates embed.FS

var tmpl *template.Template
var db *pgxpool.Pool

func main() {
	var err error
	tmpl, err = template.ParseFS(templates, "templates/*.html")
	if err != nil {
		log.Fatal("parse templates:", err)
	}

	connString := os.Getenv("XTDB_CONN_STRING")
	if connString == "" {
		connString = "postgres://localhost:5432/xtdb?sslmode=disable"
	}

	db, err = pgxpool.New(context.Background(), connString)
	if err != nil {
		log.Fatal("connect to XTDB:", err)
	}
	defer db.Close()

	http.HandleFunc("/", handleIndex)
	http.HandleFunc("/api/processes", handleProcesses)
	http.HandleFunc("/api/activities/", handleProcessActivities)
	http.HandleFunc("/api/audit/misrouted", handleMisroutedAudit)
	http.HandleFunc("/api/audit/counterfactual", handleCounterfactual)
	http.HandleFunc("/api/timeline", handleTimeline)
	http.HandleFunc("/api/audit/before-after", handleBeforeAfter)
	http.HandleFunc("/api/activity/churn-signals", handleActivityChurnSignals)
	http.HandleFunc("/api/activity/routing-decisions", handleActivityRoutingDecisions)
	http.HandleFunc("/architecture.svg", handleArchitectureSVG)
	http.HandleFunc("/fluxnova-ui.png", handleFluxnovaUI)
	http.HandleFunc("/api/export", handleExport)

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Printf("Starting Decision Observability UI on http://localhost:%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if err := tmpl.ExecuteTemplate(w, "index.html", nil); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func handleProcesses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := db.Query(ctx, `
		SELECT process_instance_id, process_definition_key, business_key, state,
			start_time, end_time, duration_millis
		FROM fluxnova_processes
		ORDER BY _valid_from DESC
		LIMIT 50
	`)
	if err != nil {
		jsonResponse(w, []map[string]any{})
		return
	}
	defer rows.Close()

	var processes []map[string]any
	for rows.Next() {
		var processID, defKey, state string
		var businessKey *string
		var startTime string
		var endTime *string
		var durationMillis *int64

		if err := rows.Scan(&processID, &defKey, &businessKey, &state, &startTime, &endTime, &durationMillis); err != nil {
			continue
		}

		proc := map[string]any{
			"process_instance_id":    processID,
			"process_definition_key": defKey,
			"state":                  state,
			"start_time":             startTime,
		}
		if businessKey != nil {
			proc["business_key"] = *businessKey
		}
		if endTime != nil {
			proc["end_time"] = *endTime
		}
		if durationMillis != nil {
			proc["duration_millis"] = *durationMillis
		}
		processes = append(processes, proc)
	}

	jsonResponse(w, processes)
}

func handleProcessActivities(w http.ResponseWriter, r *http.Request) {
	processID := strings.TrimPrefix(r.URL.Path, "/api/activities/")
	if processID == "" {
		jsonError(w, fmt.Errorf("process_instance_id required"), 400)
		return
	}

	ctx := r.Context()
	sql := fmt.Sprintf(`
		SELECT _id, activity_id, activity_name, activity_type, execution_id,
			start_time, end_time, duration_millis, canceled, process_variables
		FROM fluxnova_events
		WHERE process_instance_id = %s
		ORDER BY start_time
	`, quote(processID))

	rows, err := db.Query(ctx, sql)
	if err != nil {
		jsonResponse(w, []map[string]any{})
		return
	}
	defer rows.Close()

	var activities []map[string]any
	for rows.Next() {
		var id, activityID, activityType, executionID, startTime string
		var activityName, endTime, processVars *string
		var durationMillis *int64
		var canceled bool

		if err := rows.Scan(&id, &activityID, &activityName, &activityType, &executionID,
			&startTime, &endTime, &durationMillis, &canceled, &processVars); err != nil {
			continue
		}

		act := map[string]any{
			"id":            id,
			"activity_id":   activityID,
			"activity_type": activityType,
			"execution_id":  executionID,
			"start_time":    startTime,
			"canceled":      canceled,
		}
		if activityName != nil {
			act["activity_name"] = *activityName
		}
		if endTime != nil {
			act["end_time"] = *endTime
		}
		if durationMillis != nil {
			act["duration_millis"] = *durationMillis
		}
		if processVars != nil {
			var vars map[string]any
			if json.Unmarshal([]byte(*processVars), &vars) == nil {
				act["variables"] = vars
			}
		}
		activities = append(activities, act)
	}

	jsonResponse(w, activities)
}

func handleMisroutedAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sql := `
		SELECT
			p.process_instance_id,
			CAST(p.start_time AS TIMESTAMPTZ) as decision_time,
			c.customer_id,
			c.churn_score,
			c.risk_level,
			c._valid_from as churn_flagged_at
		FROM fluxnova_processes p
		JOIN churn_predictions FOR VALID_TIME ALL AS c
			ON c.customer_id = p.business_key
		WHERE c.churn_score > 0.7
		  AND c._valid_from <= CAST(p.start_time AS TIMESTAMPTZ)
		  AND (c._valid_to IS NULL OR c._valid_to > CAST(p.start_time AS TIMESTAMPTZ))
		ORDER BY CAST(p.start_time AS TIMESTAMPTZ) DESC
	`

	rows, err := db.Query(ctx, sql)
	if err != nil {
		jsonResponse(w, map[string]any{"count": 0, "results": []any{}})
		return
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var processID, customerID, riskLevel string
		var decisionTime, churnFlaggedAt time.Time
		var churnScore float64

		if err := rows.Scan(&processID, &decisionTime, &customerID, &churnScore, &riskLevel, &churnFlaggedAt); err != nil {
			continue
		}

		results = append(results, map[string]any{
			"process_instance_id": processID,
			"decision_time":       decisionTime.Format(time.RFC3339),
			"customer_id":         customerID,
			"churn_score":         churnScore,
			"risk_level":          riskLevel,
			"churn_flagged_at":    churnFlaggedAt.Format(time.RFC3339),
			"lag_seconds":         decisionTime.Sub(churnFlaggedAt).Seconds(),
		})
	}

	jsonResponse(w, map[string]any{
		"description": "Customers with high churn risk at the time of process start",
		"count":       len(results),
		"results":     results,
	})
}

func handleCounterfactual(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sql := `
		SELECT
			c.customer_id,
			c.churn_score as score_at_model_time,
			o.churned,
			o.churn_reason,
			o.final_ltv
		FROM churn_predictions c
		JOIN customer_outcomes o ON o.customer_id = c.customer_id
		WHERE c.churn_score > 0.7
		ORDER BY c.churn_score DESC
	`

	rows, err := db.Query(ctx, sql)
	if err != nil {
		jsonResponse(w, map[string]any{"count": 0, "results": []any{}})
		return
	}
	defer rows.Close()

	var results []map[string]any
	var totalLostLTV float64
	var churned, retained int

	for rows.Next() {
		var customerID string
		var churnScore, finalLTV float64
		var didChurn bool
		var churnReason *string

		if err := rows.Scan(&customerID, &churnScore, &didChurn, &churnReason, &finalLTV); err != nil {
			continue
		}

		reason := ""
		if churnReason != nil {
			reason = *churnReason
		}

		if didChurn {
			churned++
			totalLostLTV += finalLTV
		} else {
			retained++
		}

		results = append(results, map[string]any{
			"customer_id":  customerID,
			"churn_score":  churnScore,
			"churned":      didChurn,
			"churn_reason": reason,
			"final_ltv":    finalLTV,
		})
	}

	jsonResponse(w, map[string]any{
		"description":     "High-risk customers (score > 0.7) and their actual outcomes",
		"high_risk_total": len(results),
		"churned":         churned,
		"retained":        retained,
		"total_lost_ltv":  totalLostLTV,
		"results":         results,
	})
}

func handleTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sql := `
		SELECT
			'process' as source,
			process_instance_id as id,
			CAST(start_time AS TIMESTAMPTZ) as event_time,
			state as description,
			_system_from as recorded_at
		FROM fluxnova_processes
		UNION ALL
		SELECT
			'churn_prediction' as source,
			customer_id as id,
			_valid_from as event_time,
			'Churn score: ' || CAST(churn_score AS VARCHAR) as description,
			_system_from as recorded_at
		FROM churn_predictions
		UNION ALL
		SELECT
			'outcome' as source,
			customer_id as id,
			_valid_from as event_time,
			CASE WHEN churned THEN 'CHURNED: ' || COALESCE(churn_reason, 'unknown') ELSE 'Retained' END as description,
			_system_from as recorded_at
		FROM customer_outcomes
		ORDER BY event_time DESC
		LIMIT 100
	`

	rows, err := db.Query(ctx, sql)
	if err != nil {
		jsonResponse(w, []any{})
		return
	}
	defer rows.Close()

	var timeline []map[string]any
	for rows.Next() {
		var source, id, description string
		var eventTime, recordedAt time.Time

		if err := rows.Scan(&source, &id, &eventTime, &description, &recordedAt); err != nil {
			continue
		}

		timeline = append(timeline, map[string]any{
			"source":      source,
			"id":          id,
			"event_time":  eventTime.Format(time.RFC3339),
			"description": description,
			"recorded_at": recordedAt.Format(time.RFC3339),
		})
	}

	jsonResponse(w, timeline)
}

func handleBeforeAfter(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]any{
		"error": "Corrections not loaded yet. Run: go run ./demo/loaders/corrections.go",
	})
}

func handleActivityChurnSignals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := db.Query(ctx, `
		SELECT _id, process_instance_id, activity_id, customer_id,
			churn_score, risk_level, signal_source, last_activity_at, _valid_from
		FROM activity_churn_signals
		ORDER BY _valid_from DESC
		LIMIT 50
	`)
	if err != nil {
		jsonResponse(w, []map[string]any{})
		return
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id string
		var processID, activityID, customerID *string
		var churnScore *float64
		var riskLevel, signalSource, lastActivityAt *string
		var validFrom time.Time

		if err := rows.Scan(&id, &processID, &activityID, &customerID,
			&churnScore, &riskLevel, &signalSource, &lastActivityAt, &validFrom); err != nil {
			continue
		}

		result := map[string]any{
			"id":         id,
			"valid_from": validFrom.Format(time.RFC3339),
		}
		if processID != nil {
			result["process_instance_id"] = *processID
		}
		if activityID != nil {
			result["activity_id"] = *activityID
		}
		if customerID != nil {
			result["customer_id"] = *customerID
		}
		if churnScore != nil {
			result["churn_score"] = *churnScore
		}
		if riskLevel != nil {
			result["risk_level"] = *riskLevel
		}

		results = append(results, result)
	}

	jsonResponse(w, results)
}

func handleActivityRoutingDecisions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rows, err := db.Query(ctx, `
		SELECT _id, process_instance_id, activity_id, ticket_id, customer_id,
			queue, priority, reason_codes, escalation_level,
			input_churn_score, input_churn_risk, input_sentiment, input_urgency,
			input_customer_tier, input_customer_ltv, _valid_from
		FROM activity_routing_decisions
		ORDER BY _valid_from DESC
		LIMIT 50
	`)
	if err != nil {
		jsonResponse(w, []map[string]any{})
		return
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id string
		var processID, activityID, ticketID, customerID *string
		var queue *string
		var priority, escalationLevel *int
		var reasonCodes *string
		var inputChurnScore, inputCustomerLTV *float64
		var inputChurnRisk, inputSentiment, inputUrgency, inputCustomerTier *string
		var validFrom time.Time

		if err := rows.Scan(&id, &processID, &activityID, &ticketID, &customerID,
			&queue, &priority, &reasonCodes, &escalationLevel,
			&inputChurnScore, &inputChurnRisk, &inputSentiment, &inputUrgency,
			&inputCustomerTier, &inputCustomerLTV, &validFrom); err != nil {
			continue
		}

		result := map[string]any{
			"id":         id,
			"valid_from": validFrom.Format(time.RFC3339),
		}
		if processID != nil {
			result["process_instance_id"] = *processID
		}
		if queue != nil {
			result["queue"] = *queue
		}
		if priority != nil {
			result["priority"] = *priority
		}
		if reasonCodes != nil {
			result["reason_codes"] = *reasonCodes
		}
		if inputChurnScore != nil {
			result["input_churn_score"] = *inputChurnScore
		}

		results = append(results, result)
	}

	jsonResponse(w, results)
}

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func quote(s string) string {
	escaped := strings.ReplaceAll(s, "'", "''")
	return "'" + escaped + "'"
}

//go:embed static/architecture.svg
var architectureSVG []byte

//go:embed static/fluxnova-ui.png
var fluxnovaUIPNG []byte

func handleArchitectureSVG(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Write(architectureSVG)
}

func handleFluxnovaUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/png")
	w.Write(fluxnovaUIPNG)
}

func handleExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	exportData := make(map[string]any)

	processes, _ := getProcessesData(ctx)
	exportData["processes"] = processes

	timeline, _ := getTimelineData(ctx)
	exportData["timeline"] = timeline

	misrouted, _ := getMisroutedData(ctx)
	exportData["misrouted"] = misrouted

	counterfactual, _ := getCounterfactualData(ctx)
	exportData["counterfactual"] = counterfactual

	churnSignals, _ := getChurnSignalsData(ctx)
	exportData["churnSignals"] = churnSignals

	routingDecisions, _ := getRoutingDecisionsData(ctx)
	exportData["routingDecisions"] = routingDecisions

	dataJSON, _ := json.Marshal(exportData)

	templateContent, err := templates.ReadFile("templates/index.html")
	if err != nil {
		jsonError(w, fmt.Errorf("read template: %w", err), 500)
		return
	}

	html := string(templateContent)

	// Inline the architecture SVG
	html = strings.Replace(html,
		`<img src="/architecture.svg" alt="Decision Observability Architecture" style="width: 100%; height: auto;">`,
		string(architectureSVG),
		1)

	// Inline the Fluxnova UI PNG as base64
	fluxnovaPNGBase64 := "data:image/png;base64," + base64.StdEncoding.EncodeToString(fluxnovaUIPNG)
	html = strings.Replace(html,
		`<img src="/fluxnova-ui.png" alt="Fluxnova BPMN Process" style="width: 100%; height: auto; border-radius: 6px; border: 1px solid var(--border);">`,
		fmt.Sprintf(`<img src="%s" alt="Fluxnova BPMN Process" style="width: 100%%; height: auto; border-radius: 6px; border: 1px solid var(--border);">`, fluxnovaPNGBase64),
		1)

	// Remove export tab
	html = strings.Replace(html,
		`<div class="tab" data-tab="export" style="margin-left: auto; background: var(--accent); color: white;">Export Static HTML</div>`,
		``,
		1)

	// Remove export tab content
	exportTabStart := strings.Index(html, `<div id="tab-export"`)
	if exportTabStart != -1 {
		exportTabEnd := strings.Index(html[exportTabStart:], `</div>
        </div>

    <script>`)
		if exportTabEnd != -1 {
			html = html[:exportTabStart] + html[exportTabStart+exportTabEnd+len(`</div>
        </div>`):]
		}
	}

	// Inject pre-loaded data
	jsInjection := fmt.Sprintf(`
    <script>
        const STATIC_EXPORT = true;
        const PRELOADED_DATA = %s;
    </script>
    <script>`, string(dataJSON))

	html = strings.Replace(html, `<script>`, jsInjection, 1)

	// Replace loadData function
	html = strings.Replace(html,
		`async function loadData() {
            try {
                const [processes, misrouted, counterfactual, timeline, churnSignals, routingDecisions] = await Promise.all([
                    fetch('/api/processes').then(r => r.json()),
                    fetch('/api/audit/misrouted').then(r => r.json()),
                    fetch('/api/audit/counterfactual').then(r => r.json()),
                    fetch('/api/timeline').then(r => r.json()),
                    fetch('/api/activity/churn-signals').then(r => r.json()),
                    fetch('/api/activity/routing-decisions').then(r => r.json())
                ]);`,
		`async function loadData() {
            try {
                const processes = PRELOADED_DATA.processes || [];
                const misrouted = PRELOADED_DATA.misrouted || {};
                const counterfactual = PRELOADED_DATA.counterfactual || {};
                const timeline = PRELOADED_DATA.timeline || [];
                const churnSignals = PRELOADED_DATA.churnSignals || [];
                const routingDecisions = PRELOADED_DATA.routingDecisions || [];`,
		1)

	// Disable auto-refresh
	html = strings.Replace(html,
		`setInterval(loadData, 5000);`,
		`// Auto-refresh disabled in static export`,
		1)

	// Update title
	html = strings.Replace(html,
		`<title>Decision Observability - Fluxnova + XTDB</title>`,
		`<title>Decision Observability Export - Fluxnova + XTDB</title>`,
		1)

	w.Header().Set("Content-Type", "text/html")
	w.Header().Set("Content-Disposition", "attachment; filename=decision-observability-export.html")
	w.Write([]byte(html))
}

func getProcessesData(ctx context.Context) ([]map[string]any, error) {
	rows, err := db.Query(ctx, `
		SELECT process_instance_id, process_definition_key, business_key, state,
			start_time, end_time, duration_millis
		FROM fluxnova_processes
		ORDER BY _valid_from DESC
		LIMIT 50
	`)
	if err != nil {
		return []map[string]any{}, nil
	}
	defer rows.Close()

	var processes []map[string]any
	for rows.Next() {
		var processID, defKey, state string
		var businessKey *string
		var startTime string
		var endTime *string
		var durationMillis *int64

		if err := rows.Scan(&processID, &defKey, &businessKey, &state, &startTime, &endTime, &durationMillis); err != nil {
			continue
		}

		proc := map[string]any{
			"process_instance_id":    processID,
			"process_definition_key": defKey,
			"state":                  state,
			"start_time":             startTime,
		}
		if businessKey != nil {
			proc["business_key"] = *businessKey
		}
		if endTime != nil {
			proc["end_time"] = *endTime
		}
		processes = append(processes, proc)
	}
	return processes, nil
}

func getTimelineData(ctx context.Context) ([]map[string]any, error) {
	rows, err := db.Query(ctx, `
		SELECT
			'process' as source,
			process_instance_id as id,
			CAST(start_time AS TIMESTAMPTZ) as event_time,
			state as description,
			_system_from as recorded_at
		FROM fluxnova_processes
		UNION ALL
		SELECT
			'churn_prediction' as source,
			customer_id as id,
			_valid_from as event_time,
			'Churn score: ' || CAST(churn_score AS VARCHAR) as description,
			_system_from as recorded_at
		FROM churn_predictions
		UNION ALL
		SELECT
			'outcome' as source,
			customer_id as id,
			_valid_from as event_time,
			CASE WHEN churned THEN 'CHURNED: ' || COALESCE(churn_reason, 'unknown') ELSE 'Retained' END as description,
			_system_from as recorded_at
		FROM customer_outcomes
		ORDER BY event_time DESC
		LIMIT 100
	`)
	if err != nil {
		return []map[string]any{}, nil
	}
	defer rows.Close()

	var timeline []map[string]any
	for rows.Next() {
		var source, id, description string
		var eventTime, recordedAt time.Time

		if err := rows.Scan(&source, &id, &eventTime, &description, &recordedAt); err != nil {
			continue
		}

		timeline = append(timeline, map[string]any{
			"source":      source,
			"id":          id,
			"event_time":  eventTime.Format(time.RFC3339),
			"description": description,
			"recorded_at": recordedAt.Format(time.RFC3339),
		})
	}
	return timeline, nil
}

func getMisroutedData(ctx context.Context) (map[string]any, error) {
	rows, err := db.Query(ctx, `
		SELECT
			p.process_instance_id,
			CAST(p.start_time AS TIMESTAMPTZ) as decision_time,
			c.customer_id,
			c.churn_score,
			c.risk_level,
			c._valid_from as churn_flagged_at
		FROM fluxnova_processes p
		JOIN churn_predictions FOR VALID_TIME ALL AS c
			ON c.customer_id = p.business_key
		WHERE c.churn_score > 0.7
		  AND c._valid_from <= CAST(p.start_time AS TIMESTAMPTZ)
		  AND (c._valid_to IS NULL OR c._valid_to > CAST(p.start_time AS TIMESTAMPTZ))
		ORDER BY CAST(p.start_time AS TIMESTAMPTZ) DESC
	`)
	if err != nil {
		return map[string]any{"count": 0, "results": []any{}}, nil
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var processID, customerID, riskLevel string
		var decisionTime, churnFlaggedAt time.Time
		var churnScore float64

		if err := rows.Scan(&processID, &decisionTime, &customerID, &churnScore, &riskLevel, &churnFlaggedAt); err != nil {
			continue
		}

		results = append(results, map[string]any{
			"process_instance_id": processID,
			"decision_time":       decisionTime.Format(time.RFC3339),
			"customer_id":         customerID,
			"churn_score":         churnScore,
			"risk_level":          riskLevel,
			"churn_flagged_at":    churnFlaggedAt.Format(time.RFC3339),
			"lag_seconds":         decisionTime.Sub(churnFlaggedAt).Seconds(),
		})
	}

	return map[string]any{
		"count":   len(results),
		"results": results,
	}, nil
}

func getCounterfactualData(ctx context.Context) (map[string]any, error) {
	rows, err := db.Query(ctx, `
		SELECT
			c.customer_id,
			c.churn_score as score_at_model_time,
			o.churned,
			o.churn_reason,
			o.final_ltv
		FROM churn_predictions c
		JOIN customer_outcomes o ON o.customer_id = c.customer_id
		WHERE c.churn_score > 0.7
		ORDER BY c.churn_score DESC
	`)
	if err != nil {
		return map[string]any{"count": 0, "results": []any{}}, nil
	}
	defer rows.Close()

	var results []map[string]any
	var totalLostLTV float64
	var churned, retained int

	for rows.Next() {
		var customerID string
		var churnScore, finalLTV float64
		var didChurn bool
		var churnReason *string

		if err := rows.Scan(&customerID, &churnScore, &didChurn, &churnReason, &finalLTV); err != nil {
			continue
		}

		reason := ""
		if churnReason != nil {
			reason = *churnReason
		}

		if didChurn {
			churned++
			totalLostLTV += finalLTV
		} else {
			retained++
		}

		results = append(results, map[string]any{
			"customer_id":  customerID,
			"churn_score":  churnScore,
			"churned":      didChurn,
			"churn_reason": reason,
			"final_ltv":    finalLTV,
		})
	}

	return map[string]any{
		"high_risk_total": len(results),
		"churned":         churned,
		"retained":        retained,
		"total_lost_ltv":  totalLostLTV,
		"results":         results,
	}, nil
}

func getChurnSignalsData(ctx context.Context) ([]map[string]any, error) {
	rows, err := db.Query(ctx, `
		SELECT _id, process_instance_id, activity_id, customer_id,
			churn_score, risk_level, signal_source, last_activity_at, _valid_from
		FROM activity_churn_signals
		ORDER BY _valid_from DESC
		LIMIT 50
	`)
	if err != nil {
		return []map[string]any{}, nil
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id string
		var processID, activityID, customerID *string
		var churnScore *float64
		var riskLevel, signalSource, lastActivityAt *string
		var validFrom time.Time

		if err := rows.Scan(&id, &processID, &activityID, &customerID,
			&churnScore, &riskLevel, &signalSource, &lastActivityAt, &validFrom); err != nil {
			continue
		}

		result := map[string]any{
			"id":         id,
			"valid_from": validFrom.Format(time.RFC3339),
		}
		if customerID != nil {
			result["customer_id"] = *customerID
		}
		if churnScore != nil {
			result["churn_score"] = *churnScore
		}
		if riskLevel != nil {
			result["risk_level"] = *riskLevel
		}
		results = append(results, result)
	}
	return results, nil
}

func getRoutingDecisionsData(ctx context.Context) ([]map[string]any, error) {
	rows, err := db.Query(ctx, `
		SELECT _id, process_instance_id, activity_id, ticket_id, customer_id,
			queue, priority, reason_codes, escalation_level, _valid_from
		FROM activity_routing_decisions
		ORDER BY _valid_from DESC
		LIMIT 50
	`)
	if err != nil {
		return []map[string]any{}, nil
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id string
		var processID, activityID, ticketID, customerID *string
		var queue *string
		var priority, escalationLevel *int
		var reasonCodes *string
		var validFrom time.Time

		if err := rows.Scan(&id, &processID, &activityID, &ticketID, &customerID,
			&queue, &priority, &reasonCodes, &escalationLevel, &validFrom); err != nil {
			continue
		}

		result := map[string]any{
			"id":         id,
			"valid_from": validFrom.Format(time.RFC3339),
		}
		if queue != nil {
			result["queue"] = *queue
		}
		if priority != nil {
			result["priority"] = *priority
		}
		if reasonCodes != nil {
			result["reason_codes"] = *reasonCodes
		}
		results = append(results, result)
	}
	return results, nil
}
