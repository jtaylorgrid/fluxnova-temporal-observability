package workflow

// TicketInput is the input for a customer service ticket
type TicketInput struct {
	TicketID    string `json:"ticketId"`
	CustomerID  string `json:"customerId"`
	Subject     string `json:"subject"`
	Body        string `json:"body"`
	Channel     string `json:"channel"`
	SubmittedAt string `json:"submittedAt"`
}

// SentimentResult is the result of sentiment analysis
type SentimentResult struct {
	Sentiment  string  `json:"sentiment"`
	Confidence float64 `json:"confidence"`
	Urgency    string  `json:"urgency"`
}

// CustomerProfile contains customer data
type CustomerProfile struct {
	CustomerID     string  `json:"customerId"`
	Tier           string  `json:"tier"`
	LTV            float64 `json:"ltv"`
	AccountAgeDays int     `json:"accountAgeDays"`
	OpenTickets    int     `json:"openTickets"`
}

// ChurnSignals contains churn prediction data
type ChurnSignals struct {
	CustomerID     string  `json:"customerId"`
	ChurnScore     float64 `json:"churnScore"`
	RiskLevel      string  `json:"riskLevel"`
	LastActivityAt string  `json:"lastActivityAt"`
	SignalSource   string  `json:"signalSource"`
}

// RoutingDecision is the result of the routing decision
type RoutingDecision struct {
	Queue           string   `json:"queue"`
	Priority        int      `json:"priority"`
	ReasonCodes     []string `json:"reasonCodes"`
	EscalationLevel int      `json:"escalationLevel"`
}

// ResponseDraft is the generated response
type ResponseDraft struct {
	Body         string `json:"body"`
	Tone         string `json:"tone"`
	SuggestHuman bool   `json:"suggestHuman"`
}
