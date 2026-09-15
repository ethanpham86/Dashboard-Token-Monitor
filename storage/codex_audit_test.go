package storage_test

import (
	"testing"
	"tokenmonitor/collector"
)

type auditFullCodexProvider struct{ rows []collector.CodexSessionDTO }

func (p auditFullCodexProvider) Dashboard(string) collector.CodexDashboardDTO {
	return collector.CodexDashboardDTO{Sessions: p.rows[:1]}
}
func (p auditFullCodexProvider) DashboardForAggregation(string) collector.CodexDashboardDTO {
	return collector.CodexDashboardDTO{Sessions: p.rows}
}

func TestCodexAuditLeaderboardUsesFullExactSessionCounters(t *testing.T) {
	s, cleanup := newCanonicalTestStorage(t, "codex_audit.db")
	defer cleanup()
	p := auditFullCodexProvider{rows: []collector.CodexSessionDTO{
		{SessionID: "a", Workspace: "project_a", Model: "gpt-test", TotalTokens: 100, InputTokens: 90, CachedInputTokens: 80, OutputTokens: 10, ModelCalls: 3, Turns: 1},
		{SessionID: "b", Workspace: "project_b", Model: "gpt-test", TotalTokens: 100, InputTokens: 10, OutputTokens: 90, ModelCalls: 5, Turns: 2},
	}}
	d, err := s.GetProjectsLeaderboard("all", "tokens", p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Projects) != 2 || d.KPIs.OpenAITotalTokens != 200 || d.KPIs.GrandTotalCalls != 8 {
		t.Fatalf("UI row limit/call mismatch: %+v", d)
	}
	for _, row := range d.Projects {
		b := row.OpenAIBreakdown
		if b.CachedTokens > 0 {
			if b.PromptTokens != 90 || b.OutputTokens != 10 || b.CachedTokens != 80 {
				t.Fatalf("session counters redistributed: %+v", b)
			}
		} else if b.PromptTokens != 10 || b.OutputTokens != 90 {
			t.Fatalf("session counters redistributed: %+v", b)
		}
	}
}
