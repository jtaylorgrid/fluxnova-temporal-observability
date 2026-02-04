package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"
)

type StartProcessRequest struct {
	Variables  map[string]Variable `json:"variables"`
	BusinessKey string             `json:"businessKey,omitempty"`
}

type Variable struct {
	Value any    `json:"value"`
	Type  string `json:"type,omitempty"`
}

var sampleTickets = []struct {
	CustomerID string
	Subject    string
	Body       string
	Channel    string
}{
	{
		CustomerID: "CUST-001",
		Subject:    "Cancel my subscription immediately",
		Body:       "I am extremely frustrated with your service. I've been trying to cancel for weeks and nobody responds. This is the worst customer service I've ever experienced. I want a full refund ASAP!",
		Channel:    "email",
	},
	{
		CustomerID: "CUST-002",
		Subject:    "Question about billing",
		Body:       "Hi, I noticed a charge on my account that I don't recognize. Could you please help me understand what it's for? Thank you.",
		Channel:    "chat",
	},
	{
		CustomerID: "CUST-003",
		Subject:    "URGENT: System down",
		Body:       "Our production system is completely down! This is a critical emergency. We need immediate assistance. This is causing us significant business impact.",
		Channel:    "phone",
	},
	{
		CustomerID: "CUST-004",
		Subject:    "Great experience - thank you!",
		Body:       "I just wanted to say thank you for the excellent support I received yesterday. The agent was very helpful and resolved my issue quickly. I really appreciate your great service!",
		Channel:    "email",
	},
	{
		CustomerID: "CUST-005",
		Subject:    "Feature request",
		Body:       "I love your product but would really appreciate it if you could add dark mode. Many of us work late and it would be much easier on our eyes. Thanks for considering this!",
		Channel:    "web",
	},
	{
		CustomerID: "CUST-006",
		Subject:    "Disappointed with recent changes",
		Body:       "The recent update has made the app terrible. I'm very disappointed and considering switching to a competitor. Please fix the performance issues.",
		Channel:    "email",
	},
	{
		CustomerID: "CUST-007",
		Subject:    "Account access issue",
		Body:       "I can't log into my account. I've tried resetting my password multiple times but nothing works. Can someone please help?",
		Channel:    "chat",
	},
	{
		CustomerID: "CUST-008",
		Subject:    "Refund request",
		Body:       "I would like to request a refund for my last purchase. The product did not meet my expectations and I'm not satisfied with it.",
		Channel:    "email",
	},
}

func main() {
	baseURL := os.Getenv("FLUXNOVA_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:18080/engine-rest"
	}

	count := 5
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%d", &count)
	}

	log.Printf("Starting %d customer service ticket processes...", count)

	for i := 0; i < count; i++ {
		ticket := sampleTickets[rand.Intn(len(sampleTickets))]
		ticketID := fmt.Sprintf("TKT-%03d", i+1)

		ticketJSON, _ := json.Marshal(map[string]string{
			"ticketId":    ticketID,
			"customerId":  ticket.CustomerID,
			"subject":     ticket.Subject,
			"body":        ticket.Body,
			"channel":     ticket.Channel,
			"submittedAt": time.Now().Format(time.RFC3339),
		})

		req := StartProcessRequest{
			BusinessKey: ticketID,
			Variables: map[string]Variable{
				"ticket":     {Value: string(ticketJSON), Type: "String"},
				"ticketId":   {Value: ticketID, Type: "String"},
				"customerId": {Value: ticket.CustomerID, Type: "String"},
				"subject":    {Value: ticket.Subject, Type: "String"},
				"body":       {Value: ticket.Body, Type: "String"},
				"channel":    {Value: ticket.Channel, Type: "String"},
			},
		}

		body, _ := json.Marshal(req)
		resp, err := http.Post(
			baseURL+"/process-definition/key/customer-service-ticket/start",
			"application/json",
			bytes.NewReader(body),
		)
		if err != nil {
			log.Printf("Failed to start process for %s: %v", ticketID, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			log.Printf("Failed to start process for %s: status %d", ticketID, resp.StatusCode)
			continue
		}

		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		log.Printf("Started process for %s: %s (customer: %s)", ticketID, result["id"], ticket.CustomerID)

		time.Sleep(500 * time.Millisecond)
	}

	log.Printf("Done! Started %d processes", count)
}
