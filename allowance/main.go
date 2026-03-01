package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"allowance/internal/config"
	"allowance/internal/db"
	"allowance/internal/privacy"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/oklog/ulid/v2"
)

//go:embed web/*
var webAssets embed.FS

type App struct {
	db      *sql.DB
	cfg     *config.Config
	privacy *privacy.Client

	mu              sync.Mutex
	approvalWaiters map[string]chan string
}

type Agent struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Icon      string    `json:"icon"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Policy struct {
	ID                        string    `json:"id"`
	AgentID                   string    `json:"agent_id"`
	PrivacyCardToken          string    `json:"privacy_card_token"`
	SpendLimit                float64   `json:"spend_limit"`
	SpendLimitCents           int64     `json:"-"`
	LimitPeriod               string    `json:"limit_period"`
	CategoryLock              []string  `json:"category_lock"`
	MerchantLock              []string  `json:"merchant_lock"`
	RequireApprovalAbove      *float64  `json:"require_approval_above"`
	RequireApprovalAboveCents *int64    `json:"-"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

type Transaction struct {
	ID           string     `json:"id"`
	AgentID      string     `json:"agent_id"`
	PolicyID     string     `json:"policy_id"`
	PrivacyToken string     `json:"privacy_token,omitempty"`
	Merchant     string     `json:"merchant"`
	Amount       float64    `json:"amount"`
	AmountCents  int64      `json:"-"`
	Currency     string     `json:"currency"`
	Status       string     `json:"status"`
	MCC          string     `json:"mcc,omitempty"`
	Purpose      string     `json:"purpose,omitempty"`
	RawPayload   string     `json:"raw_payload,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	SettledAt    *time.Time `json:"settled_at,omitempty"`
}

type ApprovalRequest struct {
	ID            string     `json:"id"`
	TransactionID string     `json:"transaction_id"`
	Reason        string     `json:"reason"`
	Status        string     `json:"status"`
	RequestedAt   time.Time  `json:"requested_at"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy    string     `json:"resolved_by,omitempty"`
	Resolution    string     `json:"resolution,omitempty"`
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer database.Close()

	app := &App{
		db:              database,
		cfg:             cfg,
		privacy:         privacy.NewClient(cfg.PrivacyBaseURL, cfg.PrivacyAPIKey),
		approvalWaiters: map[string]chan string{},
	}

	app.logEvent("system", "allowance v0.1 starting")
	if app.privacy.Available() {
		app.logEvent("privacy", "privacy api key detected")
	} else {
		app.logEvent("privacy", "privacy api key missing: degraded mode")
	}

	mux := http.NewServeMux()
	app.registerAPI(mux)
	app.registerWeb(mux)

	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      app.loggingMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		app.logEvent("system", "http server listening on :"+cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	go app.serveMCP()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
	app.logEvent("system", "shutdown complete")
}

func (a *App) registerWeb(mux *http.ServeMux) {
	webRoot, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(webRoot))

	mux.Handle("/app.js", fileServer)
	mux.Handle("/styles.css", fileServer)
	mux.Handle("/favicon.ico", fileServer)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && r.URL.Path != "/index.html" {
			http.NotFound(w, r)
			return
		}
		f, err := webRoot.Open("index.html")
		if err != nil {
			http.Error(w, "missing index.html", http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.Copy(w, f)
	})
}

func (a *App) registerAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/health", a.handleHealth)
	mux.HandleFunc("/api/v1/agents", a.handleAgents)
	mux.HandleFunc("/api/v1/agents/", a.handleAgentRoutes)
	mux.HandleFunc("/api/v1/transactions", a.handleTransactions)
	mux.HandleFunc("/api/v1/transactions/", a.handleTransactionRoutes)
	mux.HandleFunc("/api/v1/allowance/", a.handleAllowance)
	mux.HandleFunc("/api/v1/approvals", a.handleApprovals)
	mux.HandleFunc("/api/v1/approvals/pending", a.handlePendingApprovals)
	mux.HandleFunc("/api/v1/webhook/privacy", a.handlePrivacyWebhook)
	mux.HandleFunc("/api/v1/settings", a.handleSettings)
	mux.HandleFunc("/api/v1/logs", a.handleLogs)
}

func (a *App) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			a.logEvent("system", fmt.Sprintf("%s %s", r.Method, r.URL.Path))
		}
	})
}

func (a *App) serveMCP() {
	s := mcpserver.NewMCPServer("Allowance MCP", "0.1.0", mcpserver.WithToolCapabilities(false), mcpserver.WithRecovery())

	s.AddTool(mcp.NewTool("request_card",
		mcp.WithDescription("Returns virtual card details for an agent"),
		mcp.WithString("agent_id", mcp.Required(), mcp.Description("Agent ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agentID, err := req.RequireString("agent_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		card, err := a.mcpRequestCard(ctx, agentID)
		if err != nil {
			a.logEvent("mcp", "request_card error: "+err.Error())
			return mcp.NewToolResultError(err.Error()), nil
		}
		b, _ := json.Marshal(card)
		a.logEvent("mcp", "request_card for agent="+agentID)
		return mcp.NewToolResultText(string(b)), nil
	})

	s.AddTool(mcp.NewTool("check_allowance",
		mcp.WithDescription("Returns remaining spend allowance"),
		mcp.WithString("agent_id", mcp.Required(), mcp.Description("Agent ID")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agentID, err := req.RequireString("agent_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out, err := a.checkAllowance(ctx, agentID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		b, _ := json.Marshal(out)
		a.logEvent("mcp", "check_allowance for agent="+agentID)
		return mcp.NewToolResultText(string(b)), nil
	})

	s.AddTool(mcp.NewTool("request_approval",
		mcp.WithDescription("Escalate a purchase for human approval"),
		mcp.WithString("agent_id", mcp.Required()),
		mcp.WithString("merchant", mcp.Required()),
		mcp.WithNumber("amount", mcp.Required()),
		mcp.WithString("reason", mcp.Required()),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agentID, err := req.RequireString("agent_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		merchant, err := req.RequireString("merchant")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		amount, err := req.RequireFloat("amount")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		reason, err := req.RequireString("reason")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		approvalID, _, err := a.createApproval(ctx, agentID, merchant, amountToCents(amount), reason)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		approved, resolution, err := a.waitForApproval(ctx, approvalID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out := map[string]any{"approved": approved, "message": resolution, "approval_id": approvalID}
		b, _ := json.Marshal(out)
		a.logEvent("mcp", "request_approval resolved="+resolution)
		return mcp.NewToolResultText(string(b)), nil
	})

	s.AddTool(mcp.NewTool("log_transaction",
		mcp.WithDescription("Log a completed purchase"),
		mcp.WithString("agent_id", mcp.Required()),
		mcp.WithString("merchant", mcp.Required()),
		mcp.WithNumber("amount", mcp.Required()),
		mcp.WithString("purpose", mcp.Description("Purpose text")),
		mcp.WithString("transaction_ref", mcp.Description("Provider transaction reference")),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		agentID, err := req.RequireString("agent_id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		merchant, err := req.RequireString("merchant")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		amount, err := req.RequireFloat("amount")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		purpose, _ := req.RequireString("purpose")
		ref, _ := req.RequireString("transaction_ref")

		txID, err := a.insertTransaction(ctx, agentID, merchant, amountToCents(amount), "approved", "", purpose, ref)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out := map[string]any{"logged": true, "transaction_id": txID}
		b, _ := json.Marshal(out)
		a.logEvent("mcp", "log_transaction for agent="+agentID)
		return mcp.NewToolResultText(string(b)), nil
	})

	if err := mcpserver.ServeStdio(s); err != nil {
		a.logEvent("mcp", "stdio server stopped: "+err.Error())
	}
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"time":              time.Now().UTC(),
		"privacy_connected": a.privacy.Available(),
	})
}

func (a *App) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		agents, err := a.listAgents(r.Context())
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, "list_agents_failed", err)
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
	case http.MethodPost:
		var in struct {
			Name string `json:"name"`
			Icon string `json:"icon"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.writeErr(w, http.StatusBadRequest, "invalid_json", err)
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			a.writeErr(w, http.StatusBadRequest, "name_required", errors.New("name is required"))
			return
		}
		if strings.TrimSpace(in.Icon) == "" {
			in.Icon = randomIcon()
		}
		agent, policy, err := a.createAgent(r.Context(), strings.TrimSpace(in.Name), strings.TrimSpace(in.Icon))
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, "create_agent_failed", err)
			return
		}
		a.logEvent("system", "created agent "+agent.Name)
		a.writeJSON(w, http.StatusCreated, map[string]any{"agent": agent, "policy": policy})
	default:
		a.writeMethodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (a *App) handleAgentRoutes(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	agentID := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			agent, err := a.getAgent(r.Context(), agentID)
			if err != nil {
				a.writeErr(w, http.StatusNotFound, "agent_not_found", err)
				return
			}
			a.writeJSON(w, http.StatusOK, map[string]any{"agent": agent})
		case http.MethodPut:
			var in struct {
				Name   string `json:"name"`
				Icon   string `json:"icon"`
				Status string `json:"status"`
			}
			if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
				a.writeErr(w, http.StatusBadRequest, "invalid_json", err)
				return
			}
			agent, err := a.updateAgent(r.Context(), agentID, in.Name, in.Icon, in.Status)
			if err != nil {
				a.writeErr(w, http.StatusBadRequest, "update_agent_failed", err)
				return
			}
			a.writeJSON(w, http.StatusOK, map[string]any{"agent": agent})
		default:
			a.writeMethodNotAllowed(w, http.MethodGet, http.MethodPut)
		}
		return
	}

	switch parts[1] {
	case "policy":
		a.handleAgentPolicy(w, r, agentID)
	case "card":
		a.handleAgentCard(w, r, agentID)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleAgentPolicy(w http.ResponseWriter, r *http.Request, agentID string) {
	switch r.Method {
	case http.MethodGet:
		policy, err := a.getPolicyByAgent(r.Context(), agentID)
		if err != nil {
			a.writeErr(w, http.StatusNotFound, "policy_not_found", err)
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"policy": policy})
	case http.MethodPut:
		var in struct {
			SpendLimit           float64  `json:"spend_limit"`
			LimitPeriod          string   `json:"limit_period"`
			CategoryLock         []string `json:"category_lock"`
			MerchantLock         []string `json:"merchant_lock"`
			RequireApprovalAbove *float64 `json:"require_approval_above"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.writeErr(w, http.StatusBadRequest, "invalid_json", err)
			return
		}
		policy, err := a.upsertPolicy(r.Context(), agentID, in.SpendLimit, in.LimitPeriod, in.CategoryLock, in.MerchantLock, in.RequireApprovalAbove)
		if err != nil {
			a.writeErr(w, http.StatusBadRequest, "update_policy_failed", err)
			return
		}
		a.logEvent("system", "updated policy for agent="+agentID)
		a.writeJSON(w, http.StatusOK, map[string]any{"policy": policy})
	default:
		a.writeMethodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (a *App) handleAgentCard(w http.ResponseWriter, r *http.Request, agentID string) {
	if r.Method != http.MethodPost {
		a.writeMethodNotAllowed(w, http.MethodPost)
		return
	}

	agent, err := a.getAgent(r.Context(), agentID)
	if err != nil {
		a.writeErr(w, http.StatusNotFound, "agent_not_found", err)
		return
	}
	policy, err := a.getPolicyByAgent(r.Context(), agentID)
	if err != nil {
		a.writeErr(w, http.StatusNotFound, "policy_not_found", err)
		return
	}

	in := privacy.CreateCardInput{
		Memo:          "Allowance " + agent.Name,
		SpendLimit:    policy.SpendLimitCents,
		SpendLimitDur: strings.ToUpper(policy.LimitPeriod),
		State:         strings.ToUpper(agent.Status),
		MerchantLock:  policy.MerchantLock,
		CategoryLock:  policy.CategoryLock,
	}
	card, err := a.privacy.CreateCard(r.Context(), in)
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, privacy.ErrPrivacyUnavailable) {
			code = http.StatusBadRequest
		}
		a.writeErr(w, code, "provision_card_failed", err)
		return
	}

	if err := a.setPolicyCardToken(r.Context(), policy.ID, card.Token); err != nil {
		a.writeErr(w, http.StatusInternalServerError, "save_card_token_failed", err)
		return
	}
	a.logEvent("privacy", "provisioned card for agent="+agent.Name)
	a.writeJSON(w, http.StatusOK, map[string]any{"card": card, "policy_id": policy.ID})
}

func (a *App) handleTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	items, err := a.listTransactions(r.Context(), r.URL.Query())
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, "list_transactions_failed", err)
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"transactions": items})
}

func (a *App) handleTransactionRoutes(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/api/v1/transactions/")
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	txID := parts[0]

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			a.writeMethodNotAllowed(w, http.MethodGet)
			return
		}
		tx, err := a.getTransaction(r.Context(), txID)
		if err != nil {
			a.writeErr(w, http.StatusNotFound, "transaction_not_found", err)
			return
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"transaction": tx})
		return
	}

	action := parts[1]
	if r.Method != http.MethodPost || (action != "approve" && action != "decline") {
		http.NotFound(w, r)
		return
	}

	status := "approved"
	resolution := "approved"
	if action == "decline" {
		status = "declined"
		resolution = "declined"
	}
	if err := a.resolveByTransaction(r.Context(), txID, status, resolution, "human"); err != nil {
		a.writeErr(w, http.StatusInternalServerError, "resolve_transaction_failed", err)
		return
	}
	a.logEvent("system", fmt.Sprintf("%s transaction=%s", action, txID))
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": status})
}

func (a *App) handleAllowance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	agentID := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/allowance/"), "/")
	if agentID == "" {
		a.writeErr(w, http.StatusBadRequest, "agent_id_required", errors.New("agent_id required"))
		return
	}
	out, err := a.checkAllowance(r.Context(), agentID)
	if err != nil {
		a.writeErr(w, http.StatusBadRequest, "allowance_check_failed", err)
		return
	}
	a.writeJSON(w, http.StatusOK, out)
}

func (a *App) handleApprovals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	var in struct {
		AgentID  string  `json:"agent_id"`
		Merchant string  `json:"merchant"`
		Amount   float64 `json:"amount"`
		Reason   string  `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid_json", err)
		return
	}
	approvalID, txID, err := a.createApproval(r.Context(), in.AgentID, in.Merchant, amountToCents(in.Amount), in.Reason)
	if err != nil {
		a.writeErr(w, http.StatusBadRequest, "create_approval_failed", err)
		return
	}
	a.logEvent("system", "approval created id="+approvalID)
	a.writeJSON(w, http.StatusCreated, map[string]any{"approval_id": approvalID, "transaction_id": txID, "status": "pending"})
}

func (a *App) handlePendingApprovals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	rows, err := a.db.QueryContext(r.Context(), `SELECT id, transaction_id, reason, status, requested_at, resolved_at, resolved_by, resolution FROM approval_requests WHERE status='pending' ORDER BY requested_at DESC`)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, "list_pending_approvals_failed", err)
		return
	}
	defer rows.Close()

	items := []ApprovalRequest{}
	for rows.Next() {
		item, err := scanApproval(rows)
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, "scan_pending_approvals_failed", err)
			return
		}
		items = append(items, item)
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"approvals": items})
}

func (a *App) handlePrivacyWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		a.writeMethodNotAllowed(w, http.MethodPost)
		return
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		a.writeErr(w, http.StatusBadRequest, "invalid_json", err)
		return
	}
	b, _ := json.Marshal(payload)
	event := stringValue(payload["event"])
	if event == "" {
		event = stringValue(payload["type"])
	}
	a.logEvent("webhook", fmt.Sprintf("privacy webhook received event=%s", event))

	if txnToken := stringValue(payload["token"]); txnToken != "" {
		status := "pending"
		switch event {
		case "transaction.settled":
			status = "approved"
		case "transaction.declined":
			status = "declined"
		}
		_, _ = a.db.ExecContext(r.Context(), `UPDATE transactions SET status=?, raw_payload_json=? WHERE privacy_token=?`, status, string(b), txnToken)
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings, err := a.getSettings(r.Context())
		if err != nil {
			a.writeErr(w, http.StatusInternalServerError, "get_settings_failed", err)
			return
		}
		a.writeJSON(w, http.StatusOK, settings)
	case http.MethodPut:
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			a.writeErr(w, http.StatusBadRequest, "invalid_json", err)
			return
		}
		if err := a.saveSettings(r.Context(), in); err != nil {
			a.writeErr(w, http.StatusBadRequest, "save_settings_failed", err)
			return
		}
		settings, _ := a.getSettings(r.Context())
		a.writeJSON(w, http.StatusOK, settings)
	default:
		a.writeMethodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (a *App) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		a.writeMethodNotAllowed(w, http.MethodGet)
		return
	}
	limit := 200
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	filterType := strings.TrimSpace(r.URL.Query().Get("type"))
	query := `SELECT id, type, message, created_at FROM log_events`
	args := []any{}
	if filterType != "" && filterType != "all" {
		query += ` WHERE type = ?`
		args = append(args, filterType)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := a.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		a.writeErr(w, http.StatusInternalServerError, "list_logs_failed", err)
		return
	}
	defer rows.Close()

	type logRow struct {
		ID        int64     `json:"id"`
		Type      string    `json:"type"`
		Message   string    `json:"message"`
		CreatedAt time.Time `json:"created_at"`
	}
	items := []logRow{}
	for rows.Next() {
		var item logRow
		var at string
		if err := rows.Scan(&item.ID, &item.Type, &item.Message, &at); err != nil {
			a.writeErr(w, http.StatusInternalServerError, "scan_logs_failed", err)
			return
		}
		item.CreatedAt = parseTime(at)
		items = append(items, item)
	}
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"logs": items})
}

func (a *App) createAgent(ctx context.Context, name, icon string) (Agent, Policy, error) {
	now := time.Now().UTC()
	agent := Agent{ID: newID(), Name: name, Icon: icon, Status: "active", CreatedAt: now, UpdatedAt: now}
	if _, err := a.db.ExecContext(ctx, `INSERT INTO agents(id,name,icon,status,created_at,updated_at) VALUES(?,?,?,?,?,?)`,
		agent.ID, agent.Name, agent.Icon, agent.Status, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return Agent{}, Policy{}, err
	}

	policy := Policy{ID: newID(), AgentID: agent.ID, SpendLimitCents: 5000, SpendLimit: 50, LimitPeriod: "monthly", CategoryLock: []string{}, MerchantLock: []string{}, CreatedAt: now, UpdatedAt: now}
	if _, err := a.db.ExecContext(ctx, `INSERT INTO policies(id,agent_id,spend_limit_cents,limit_period,category_lock_json,merchant_lock_json,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?)`,
		policy.ID, policy.AgentID, policy.SpendLimitCents, policy.LimitPeriod, "[]", "[]", now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return Agent{}, Policy{}, err
	}
	return agent, policy, nil
}

func (a *App) listAgents(ctx context.Context) ([]Agent, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id,name,icon,status,created_at,updated_at FROM agents ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Agent{}
	for rows.Next() {
		var item Agent
		var created, updated string
		if err := rows.Scan(&item.ID, &item.Name, &item.Icon, &item.Status, &created, &updated); err != nil {
			return nil, err
		}
		item.CreatedAt = parseTime(created)
		item.UpdatedAt = parseTime(updated)
		items = append(items, item)
	}
	return items, nil
}

func (a *App) getAgent(ctx context.Context, id string) (Agent, error) {
	var item Agent
	var created, updated string
	err := a.db.QueryRowContext(ctx, `SELECT id,name,icon,status,created_at,updated_at FROM agents WHERE id=?`, id).Scan(
		&item.ID, &item.Name, &item.Icon, &item.Status, &created, &updated,
	)
	if err != nil {
		return Agent{}, err
	}
	item.CreatedAt = parseTime(created)
	item.UpdatedAt = parseTime(updated)
	return item, nil
}

func (a *App) updateAgent(ctx context.Context, id, name, icon, status string) (Agent, error) {
	agent, err := a.getAgent(ctx, id)
	if err != nil {
		return Agent{}, err
	}
	if strings.TrimSpace(name) != "" {
		agent.Name = strings.TrimSpace(name)
	}
	if strings.TrimSpace(icon) != "" {
		agent.Icon = strings.TrimSpace(icon)
	}
	if strings.TrimSpace(status) != "" {
		status = strings.ToLower(strings.TrimSpace(status))
		if status != "active" && status != "paused" {
			return Agent{}, errors.New("status must be active or paused")
		}
		agent.Status = status
	}
	agent.UpdatedAt = time.Now().UTC()
	if _, err := a.db.ExecContext(ctx, `UPDATE agents SET name=?, icon=?, status=?, updated_at=? WHERE id=?`, agent.Name, agent.Icon, agent.Status, agent.UpdatedAt.Format(time.RFC3339), id); err != nil {
		return Agent{}, err
	}
	policy, _ := a.getPolicyByAgent(ctx, id)
	if policy.PrivacyCardToken != "" {
		state := "active"
		if agent.Status == "paused" {
			state = "paused"
		}
		if err := a.privacy.SetCardState(ctx, policy.PrivacyCardToken, state); err != nil && !errors.Is(err, privacy.ErrPrivacyUnavailable) {
			a.logEvent("privacy", "set card state failed: "+err.Error())
		}
	}
	return agent, nil
}

func (a *App) getPolicyByAgent(ctx context.Context, agentID string) (Policy, error) {
	var p Policy
	var cats, merchants, created, updated string
	var req sql.NullInt64
	err := a.db.QueryRowContext(ctx, `SELECT id,agent_id,privacy_card_token,spend_limit_cents,limit_period,category_lock_json,merchant_lock_json,require_approval_above_cents,created_at,updated_at FROM policies WHERE agent_id=?`, agentID).
		Scan(&p.ID, &p.AgentID, &p.PrivacyCardToken, &p.SpendLimitCents, &p.LimitPeriod, &cats, &merchants, &req, &created, &updated)
	if err != nil {
		return Policy{}, err
	}
	_ = json.Unmarshal([]byte(cats), &p.CategoryLock)
	_ = json.Unmarshal([]byte(merchants), &p.MerchantLock)
	if req.Valid {
		v := centsToAmount(req.Int64)
		p.RequireApprovalAbove = &v
		p.RequireApprovalAboveCents = &req.Int64
	}
	p.SpendLimit = centsToAmount(p.SpendLimitCents)
	p.CreatedAt = parseTime(created)
	p.UpdatedAt = parseTime(updated)
	return p, nil
}

func (a *App) upsertPolicy(ctx context.Context, agentID string, spendLimit float64, limitPeriod string, categoryLock, merchantLock []string, requireApprovalAbove *float64) (Policy, error) {
	if spendLimit <= 0 {
		spendLimit = 50
	}
	if strings.TrimSpace(limitPeriod) == "" {
		limitPeriod = "monthly"
	}
	limitPeriod = strings.ToLower(limitPeriod)
	switch limitPeriod {
	case "transaction", "daily", "monthly", "total":
	default:
		return Policy{}, errors.New("limit_period must be one of transaction|daily|monthly|total")
	}

	policy, err := a.getPolicyByAgent(ctx, agentID)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return Policy{}, err
		}
		policy = Policy{ID: newID(), AgentID: agentID, CreatedAt: time.Now().UTC()}
	}

	cats, _ := json.Marshal(nonNil(categoryLock))
	merchants, _ := json.Marshal(nonNil(merchantLock))
	var req any
	if requireApprovalAbove != nil {
		c := amountToCents(*requireApprovalAbove)
		req = c
	}
	now := time.Now().UTC().Format(time.RFC3339)

	if policy.ID == "" {
		policy.ID = newID()
	}
	if _, err := a.db.ExecContext(ctx, `INSERT INTO policies(id,agent_id,privacy_card_token,spend_limit_cents,limit_period,category_lock_json,merchant_lock_json,require_approval_above_cents,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(agent_id) DO UPDATE SET spend_limit_cents=excluded.spend_limit_cents,limit_period=excluded.limit_period,category_lock_json=excluded.category_lock_json,merchant_lock_json=excluded.merchant_lock_json,require_approval_above_cents=excluded.require_approval_above_cents,updated_at=excluded.updated_at`,
		policy.ID, agentID, policy.PrivacyCardToken, amountToCents(spendLimit), limitPeriod, string(cats), string(merchants), req, now, now,
	); err != nil {
		return Policy{}, err
	}

	return a.getPolicyByAgent(ctx, agentID)
}

func (a *App) setPolicyCardToken(ctx context.Context, policyID, token string) error {
	_, err := a.db.ExecContext(ctx, `UPDATE policies SET privacy_card_token=?, updated_at=? WHERE id=?`, token, time.Now().UTC().Format(time.RFC3339), policyID)
	return err
}

func (a *App) listTransactions(ctx context.Context, q map[string][]string) ([]Transaction, error) {
	query := `SELECT id,agent_id,policy_id,privacy_token,merchant,amount_cents,currency,status,mcc,purpose,raw_payload_json,created_at,settled_at FROM transactions`
	where := []string{}
	args := []any{}

	if agentID := first(q, "agent_id"); agentID != "" {
		where = append(where, "agent_id=?")
		args = append(args, agentID)
	}
	if status := first(q, "status"); status != "" && status != "all" {
		where = append(where, "status=?")
		args = append(args, strings.ToLower(status))
	}
	if df := first(q, "date_from"); df != "" {
		where = append(where, "created_at>=?")
		args = append(args, df)
	}
	if dt := first(q, "date_to"); dt != "" {
		where = append(where, "created_at<=?")
		args = append(args, dt)
	}
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT 200"

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := []Transaction{}
	for rows.Next() {
		item, err := scanTransaction(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (a *App) getTransaction(ctx context.Context, txID string) (Transaction, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT id,agent_id,policy_id,privacy_token,merchant,amount_cents,currency,status,mcc,purpose,raw_payload_json,created_at,settled_at FROM transactions WHERE id=?`, txID)
	if err != nil {
		return Transaction{}, err
	}
	defer rows.Close()
	if !rows.Next() {
		return Transaction{}, sql.ErrNoRows
	}
	return scanTransaction(rows)
}

func (a *App) insertTransaction(ctx context.Context, agentID, merchant string, amountCents int64, status, mcc, purpose, privacyToken string) (string, error) {
	policy, err := a.getPolicyByAgent(ctx, agentID)
	if err != nil {
		return "", err
	}
	txID := newID()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = a.db.ExecContext(ctx, `INSERT INTO transactions(id,agent_id,policy_id,privacy_token,merchant,amount_cents,currency,status,mcc,purpose,raw_payload_json,created_at,settled_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		txID, agentID, policy.ID, privacyToken, merchant, amountCents, "USD", strings.ToLower(status), mcc, purpose, "{}", now, nil,
	)
	if err != nil {
		return "", err
	}
	return txID, nil
}

func (a *App) createApproval(ctx context.Context, agentID, merchant string, amountCents int64, reason string) (string, string, error) {
	if strings.TrimSpace(agentID) == "" || strings.TrimSpace(merchant) == "" || amountCents <= 0 {
		return "", "", errors.New("agent_id, merchant, and positive amount are required")
	}
	if reason == "" {
		reason = "limit exceeded"
	}
	_, err := a.getAgent(ctx, agentID)
	if err != nil {
		return "", "", err
	}
	txID, err := a.insertTransaction(ctx, agentID, merchant, amountCents, "review", "", reason, "")
	if err != nil {
		return "", "", err
	}

	approvalID := newID()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err = a.db.ExecContext(ctx, `INSERT INTO approval_requests(id,transaction_id,reason,status,requested_at) VALUES(?,?,?,?,?)`, approvalID, txID, reason, "pending", now)
	if err != nil {
		return "", "", err
	}

	a.mu.Lock()
	a.approvalWaiters[approvalID] = make(chan string, 1)
	a.mu.Unlock()

	return approvalID, txID, nil
}

func (a *App) waitForApproval(ctx context.Context, approvalID string) (bool, string, error) {
	a.mu.Lock()
	ch, ok := a.approvalWaiters[approvalID]
	a.mu.Unlock()
	if !ok {
		return false, "unknown", errors.New("approval waiter not found")
	}

	timeout := time.Duration(a.cfg.ApprovalTimeoutMinutes) * time.Minute
	t := time.NewTimer(timeout)
	defer t.Stop()

	select {
	case <-ctx.Done():
		return false, "cancelled", ctx.Err()
	case res := <-ch:
		return res == "approved", res, nil
	case <-t.C:
		if err := a.resolveApprovalByID(context.Background(), approvalID, "declined", "expired", "timeout"); err != nil {
			return false, "expired", err
		}
		return false, "expired", nil
	}
}

func (a *App) resolveByTransaction(ctx context.Context, txID, txStatus, resolution, resolvedBy string) error {
	_, err := a.db.ExecContext(ctx, `UPDATE transactions SET status=?, settled_at=? WHERE id=?`, txStatus, time.Now().UTC().Format(time.RFC3339), txID)
	if err != nil {
		return err
	}
	row := a.db.QueryRowContext(ctx, `SELECT id FROM approval_requests WHERE transaction_id=? AND status='pending' ORDER BY requested_at DESC LIMIT 1`, txID)
	var approvalID string
	if err := row.Scan(&approvalID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	return a.resolveApprovalByID(ctx, approvalID, txStatus, resolution, resolvedBy)
}

func (a *App) resolveApprovalByID(ctx context.Context, approvalID, txStatus, resolution, resolvedBy string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	var txID string
	if err := a.db.QueryRowContext(ctx, `SELECT transaction_id FROM approval_requests WHERE id=?`, approvalID).Scan(&txID); err != nil {
		return err
	}
	if _, err := a.db.ExecContext(ctx, `UPDATE approval_requests SET status='resolved', resolved_at=?, resolved_by=?, resolution=? WHERE id=?`, now, resolvedBy, resolution, approvalID); err != nil {
		return err
	}
	if _, err := a.db.ExecContext(ctx, `UPDATE transactions SET status=?, settled_at=? WHERE id=?`, txStatus, now, txID); err != nil {
		return err
	}
	a.mu.Lock()
	if ch, ok := a.approvalWaiters[approvalID]; ok {
		select {
		case ch <- resolution:
		default:
		}
		delete(a.approvalWaiters, approvalID)
	}
	a.mu.Unlock()
	return nil
}

func (a *App) checkAllowance(ctx context.Context, agentID string) (map[string]any, error) {
	policy, err := a.getPolicyByAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	from := ""
	now := time.Now().UTC()
	switch policy.LimitPeriod {
	case "daily":
		from = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	case "monthly":
		from = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	case "transaction":
		return map[string]any{"limit": policy.SpendLimit, "spent": 0.0, "remaining": policy.SpendLimit, "period": policy.LimitPeriod}, nil
	case "total":
	}

	query := `SELECT COALESCE(SUM(amount_cents),0) FROM transactions WHERE agent_id=? AND status IN ('approved','pending')`
	args := []any{agentID}
	if from != "" {
		query += ` AND created_at>=?`
		args = append(args, from)
	}
	var spentCents int64
	if err := a.db.QueryRowContext(ctx, query, args...).Scan(&spentCents); err != nil {
		return nil, err
	}
	remainingCents := policy.SpendLimitCents - spentCents
	if remainingCents < 0 {
		remainingCents = 0
	}
	return map[string]any{
		"limit":     centsToAmount(policy.SpendLimitCents),
		"spent":     centsToAmount(spentCents),
		"remaining": centsToAmount(remainingCents),
		"period":    policy.LimitPeriod,
	}, nil
}

func (a *App) getSettings(ctx context.Context) (map[string]any, error) {
	rows, err := a.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		values[k] = v
	}
	timeout := a.cfg.ApprovalTimeoutMinutes
	if v, ok := values["approval_timeout_minutes"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = n
		}
	}
	resp := map[string]any{
		"db_path":                  a.cfg.DBPath,
		"privacy_connected":        a.privacy.Available(),
		"privacy_api_key_masked":   maskedKey(a.cfg.PrivacyAPIKey),
		"notification_webhook_url": firstNonEmpty(values["notification_webhook_url"], a.cfg.NotificationWebhookURL),
		"approval_timeout_minutes": timeout,
	}
	return resp, nil
}

func (a *App) saveSettings(ctx context.Context, in map[string]any) error {
	now := time.Now().UTC().Format(time.RFC3339)
	store := func(k, v string) error {
		_, err := a.db.ExecContext(ctx, `INSERT INTO settings(key,value,updated_at) VALUES(?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, k, v, now)
		return err
	}
	if raw, ok := in["notification_webhook_url"]; ok {
		v := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if err := store("notification_webhook_url", v); err != nil {
			return err
		}
		a.cfg.NotificationWebhookURL = v
	}
	if raw, ok := in["approval_timeout_minutes"]; ok {
		n, err := intFromAny(raw)
		if err != nil || n <= 0 {
			return errors.New("approval_timeout_minutes must be a positive integer")
		}
		if err := store("approval_timeout_minutes", strconv.Itoa(n)); err != nil {
			return err
		}
		a.cfg.ApprovalTimeoutMinutes = n
	}
	return nil
}

func (a *App) mcpRequestCard(ctx context.Context, agentID string) (map[string]any, error) {
	policy, err := a.getPolicyByAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if policy.PrivacyCardToken == "" {
		return nil, errors.New("no card provisioned; call POST /api/v1/agents/{id}/card first")
	}
	allowance, err := a.checkAllowance(ctx, agentID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"card_number":     "unavailable_after_provisioning",
		"expiry":          "unavailable_after_provisioning",
		"cvv":             "unavailable_after_provisioning",
		"privacy_token":   policy.PrivacyCardToken,
		"limit_remaining": allowance["remaining"],
	}, nil
}

func (a *App) writeJSON(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func (a *App) writeErr(w http.ResponseWriter, code int, errCode string, err error) {
	a.writeJSON(w, code, map[string]any{"error": map[string]any{"code": errCode, "message": err.Error()}})
}

func (a *App) writeMethodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	a.writeErr(w, http.StatusMethodNotAllowed, "method_not_allowed", errors.New("method not allowed"))
}

func (a *App) logEvent(kind, message string) {
	_, _ = a.db.Exec(`INSERT INTO log_events(type,message,created_at) VALUES(?,?,?)`, kind, message, time.Now().UTC().Format(time.RFC3339))
	_, _ = a.db.Exec(`DELETE FROM log_events WHERE id NOT IN (SELECT id FROM log_events ORDER BY id DESC LIMIT 500)`)
}

func scanTransaction(rows *sql.Rows) (Transaction, error) {
	var t Transaction
	var created string
	var settled sql.NullString
	if err := rows.Scan(&t.ID, &t.AgentID, &t.PolicyID, &t.PrivacyToken, &t.Merchant, &t.AmountCents, &t.Currency, &t.Status, &t.MCC, &t.Purpose, &t.RawPayload, &created, &settled); err != nil {
		return Transaction{}, err
	}
	t.CreatedAt = parseTime(created)
	if settled.Valid {
		s := parseTime(settled.String)
		t.SettledAt = &s
	}
	t.Amount = centsToAmount(t.AmountCents)
	return t, nil
}

func scanApproval(rows *sql.Rows) (ApprovalRequest, error) {
	var a ApprovalRequest
	var requested string
	var resolved sql.NullString
	var resolvedBy, resolution sql.NullString
	if err := rows.Scan(&a.ID, &a.TransactionID, &a.Reason, &a.Status, &requested, &resolved, &resolvedBy, &resolution); err != nil {
		return ApprovalRequest{}, err
	}
	a.RequestedAt = parseTime(requested)
	if resolved.Valid {
		t := parseTime(resolved.String)
		a.ResolvedAt = &t
	}
	a.ResolvedBy = resolvedBy.String
	a.Resolution = resolution.String
	return a, nil
}

func newID() string {
	src := rand.New(rand.NewSource(time.Now().UnixNano()))
	return ulid.MustNew(ulid.Timestamp(time.Now()), src).String()
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Now().UTC()
	}
	return t
}

func amountToCents(v float64) int64 {
	return int64(math.Round(v * 100))
}

func centsToAmount(v int64) float64 {
	return float64(v) / 100.0
}

func first(q map[string][]string, key string) string {
	vals := q[key]
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func intFromAny(v any) (int, error) {
	switch t := v.(type) {
	case float64:
		return int(t), nil
	case int:
		return t, nil
	case string:
		return strconv.Atoi(strings.TrimSpace(t))
	default:
		return 0, fmt.Errorf("unsupported type: %T", v)
	}
}

func maskedKey(key string) string {
	k := strings.TrimSpace(key)
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "********"
	}
	return k[:4] + strings.Repeat("*", len(k)-8) + k[len(k)-4:]
}

func stringValue(v any) string {
	s, _ := v.(string)
	return s
}

func randomIcon() string {
	icons := []string{"🤖", "🍕", "🛒", "🔎", "🧪", "💳"}
	return icons[rand.Intn(len(icons))]
}

func staticFilePath(p string) string {
	return path.Clean("/" + p)
}
