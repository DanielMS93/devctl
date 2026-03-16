// Package mcp implements a minimal MCP (Model Context Protocol) stdio JSON-RPC server
// for devctl's idea pipeline tools: side_quest and main_quest.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DanielMS93/devctl/internal/claude"
	"github.com/DanielMS93/devctl/internal/idea"
	"github.com/jmoiron/sqlx"
)

// Server is the MCP stdio JSON-RPC server.
type Server struct {
	store *idea.Store
	db    *sqlx.DB
}

// NewServer creates an MCP server.
func NewServer(db *sqlx.DB) *Server {
	return &Server{
		store: idea.NewStore(db),
		db:    db,
	}
}

// jsonRPCRequest is a JSON-RPC 2.0 request.
type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// jsonRPCResponse is a JSON-RPC 2.0 response.
type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCP protocol types.
type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
}

type capabilities struct {
	Tools *toolsCap `json:"tools,omitempty"`
}

type toolsCap struct{}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	InputSchema inputSchema `json:"inputSchema"`
}

type inputSchema struct {
	Type       string              `json:"type"`
	Properties map[string]property `json:"properties"`
	Required   []string            `json:"required,omitempty"`
}

type property struct {
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Items       *itemDef `json:"items,omitempty"`
}

type itemDef struct {
	Type string `json:"type"`
}

type toolsListResult struct {
	Tools []toolDef `json:"tools"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

type toolCallResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Serve runs the MCP server on stdin/stdout.
func (s *Server) Serve(ctx context.Context) error {
	reader := bufio.NewReader(os.Stdin)
	writer := os.Stdout

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var req jsonRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			slog.Warn("mcp: invalid json-rpc", "err", err)
			continue
		}

		resp := s.handleRequest(ctx, req)
		data, _ := json.Marshal(resp)
		fmt.Fprintf(writer, "%s\n", data)
	}
}

func (s *Server) handleRequest(ctx context.Context, req jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: initializeResult{
				ProtocolVersion: "2024-11-05",
				Capabilities:    capabilities{Tools: &toolsCap{}},
				ServerInfo:      serverInfo{Name: "devctl", Version: "0.1.0"},
			},
		}

	case "notifications/initialized":
		// No response needed for notifications.
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID}

	case "tools/list":
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: toolsListResult{
				Tools: []toolDef{
					{
						Name:        "side_quest",
						Description: "Spawn a parallel Claude session in a worktree to investigate an idea. The side-quest runs independently and can be incorporated back later with main_quest.",
						InputSchema: inputSchema{
							Type: "object",
							Properties: map[string]property{
								"prompt": {
									Type:        "string",
									Description: "What to investigate or build in the side-quest",
								},
								"depends_on": {
									Type:        "array",
									Description: "Optional list of idea IDs that must complete before this one starts",
									Items:       &itemDef{Type: "string"},
								},
							},
							Required: []string{"prompt"},
						},
					},
					{
						Name:        "main_quest",
						Description: "Incorporate findings from a completed side-quest back into the current session. Merges code changes and/or returns the side-quest transcript as context.",
						InputSchema: inputSchema{
							Type: "object",
							Properties: map[string]property{
								"idea_id": {
									Type:        "string",
									Description: "ID of the completed side-quest to incorporate. If omitted, lists completed non-incorporated ideas.",
								},
							},
						},
					},
				},
			},
		}

	case "tools/call":
		var params toolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return errorResponse(req.ID, -32602, "invalid params: "+err.Error())
		}
		return s.handleToolCall(ctx, req.ID, params)

	default:
		return errorResponse(req.ID, -32601, "method not found: "+req.Method)
	}
}

func (s *Server) handleToolCall(ctx context.Context, id json.RawMessage, params toolCallParams) jsonRPCResponse {
	switch params.Name {
	case "side_quest":
		return s.handleSideQuest(ctx, id, params.Arguments)
	case "main_quest":
		return s.handleMainQuest(ctx, id, params.Arguments)
	default:
		return errorResponse(id, -32602, "unknown tool: "+params.Name)
	}
}

func (s *Server) handleSideQuest(ctx context.Context, id json.RawMessage, args map[string]any) jsonRPCResponse {
	prompt, _ := args["prompt"].(string)
	if prompt == "" {
		return toolError(id, "prompt is required")
	}

	// Auto-detect repo from cwd.
	cwd, _ := os.Getwd()
	repoPath := cwd

	// Auto-detect current branch.
	parentBranch := detectCurrentBranch(cwd)

	// Auto-detect parent session: find the newest active JSONL.
	parentSessionID := detectActiveSession(repoPath)

	// Create the idea.
	i, err := s.store.Create(ctx, prompt, repoPath, "side", parentSessionID, parentBranch)
	if err != nil {
		return toolError(id, "create idea: "+err.Error())
	}

	// Add dependencies if specified.
	if depsRaw, ok := args["depends_on"]; ok {
		if depsList, ok := depsRaw.([]any); ok {
			for _, d := range depsList {
				depID, _ := d.(string)
				if depID == "" {
					continue
				}
				dep, err := s.store.Get(ctx, depID)
				if err != nil {
					return toolError(id, fmt.Sprintf("resolve dependency %q: %v", depID, err))
				}
				if err := s.store.AddDep(ctx, i.ID, dep.ID); err != nil {
					return toolError(id, "add dependency: "+err.Error())
				}
			}
		}
	}

	msg := fmt.Sprintf("Side-quest created: %s\nID: %s\nPrompt: %s\n\nThe idea will be picked up by the devctl executor within ~5 seconds. Use main_quest to incorporate findings when complete.",
		i.ID[:8], i.ID[:8], prompt)

	return toolSuccess(id, msg)
}

func (s *Server) handleMainQuest(ctx context.Context, id json.RawMessage, args map[string]any) jsonRPCResponse {
	ideaID, _ := args["idea_id"].(string)

	// If no idea_id, list completed non-incorporated ideas.
	if ideaID == "" {
		cwd, _ := os.Getwd()
		ideas, err := s.store.ListByState(ctx, "completed")
		if err != nil {
			return toolError(id, "list ideas: "+err.Error())
		}

		var available []string
		for _, i := range ideas {
			if i.Incorporated == 1 {
				continue
			}
			if i.RepoID != cwd {
				continue
			}
			prompt := i.Prompt
			if len(prompt) > 60 {
				prompt = prompt[:57] + "..."
			}
			available = append(available, fmt.Sprintf("  %s: %s [%s]", i.ID[:8], prompt, i.State))
		}

		if len(available) == 0 {
			return toolSuccess(id, "No completed side-quests available to incorporate.")
		}

		return toolSuccess(id, "Completed side-quests available to incorporate:\n"+strings.Join(available, "\n")+"\n\nCall main_quest with idea_id to incorporate one.")
	}

	// Get the idea.
	i, err := s.store.Get(ctx, ideaID)
	if err != nil {
		return toolError(id, err.Error())
	}

	// Incorporate.
	cwd, _ := os.Getwd()
	result, err := idea.Incorporate(ctx, s.store, i, cwd)
	if err != nil {
		return toolError(id, "incorporate: "+err.Error())
	}

	var parts []string
	if result.CodeMerged {
		parts = append(parts, fmt.Sprintf("Code merged from branch %s:\n%s", i.Branch, result.MergeMsg))
	}
	if result.Transcript != "" {
		// Truncate transcript for tool result.
		transcript := result.Transcript
		if len(transcript) > 30000 {
			transcript = transcript[len(transcript)-30000:]
		}
		parts = append(parts, fmt.Sprintf("Side-quest findings:\n%s", transcript))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("Side-quest %s incorporated (no code changes or transcript found).", i.ID[:8]))
	}

	return toolSuccess(id, strings.Join(parts, "\n\n"))
}

// detectCurrentBranch returns the current git branch at the given path.
func detectCurrentBranch(repoPath string) string {
	headPath := filepath.Join(repoPath, ".git", "HEAD")
	data, err := os.ReadFile(headPath)
	if err != nil {
		return "main"
	}
	ref := strings.TrimSpace(string(data))
	if strings.HasPrefix(ref, "ref: refs/heads/") {
		return strings.TrimPrefix(ref, "ref: refs/heads/")
	}
	return "main"
}

// detectActiveSession finds the most recently active Claude session for the repo.
func detectActiveSession(repoPath string) string {
	threshold := 20 * time.Minute
	sessions, err := claude.ScanSessionsWithThreshold(repoPath, threshold)
	if err != nil || len(sessions) == 0 {
		return ""
	}
	// Return the most recent session (already sorted by LastActivity desc).
	return sessions[0].ID
}

func errorResponse(id json.RawMessage, code int, msg string) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: msg},
	}
}

func toolSuccess(id json.RawMessage, text string) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: toolCallResult{
			Content: []contentBlock{{Type: "text", Text: text}},
		},
	}
}

func toolError(id json.RawMessage, text string) jsonRPCResponse {
	return jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: toolCallResult{
			Content: []contentBlock{{Type: "text", Text: "Error: " + text}},
			IsError: true,
		},
	}
}
