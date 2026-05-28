//go:build integration

package cost_test

import (
	"context"
	"testing"
	"time"

	"github.com/aibao/server/internal/model"
	"github.com/aibao/server/internal/service/cost"
	"github.com/stretchr/testify/require"
)

func cleanupCostAndOutlineEvents(t *testing.T, eventIDs []string, outlineIDs []string) {
	t.Helper()
	db := integrationDB(t)
	for _, id := range eventIDs {
		db.Exec("DELETE FROM cost_events WHERE event_id = ?", id)
	}
	for _, id := range outlineIDs {
		db.Exec("DELETE FROM outline_events WHERE outline_id = ?", id)
		db.Exec("DELETE FROM cost_events WHERE outline_id = ?", id)
	}
}

func TestAggregator_Overall(t *testing.T) {
	db := integrationDB(t)
	storyID := int64(99001)
	eventIDs := []string{
		"agg11111:story:llm_call:1",
		"agg11111:tts:synthesize:1",
	}
	outlineIDs := []string{"ol_agg_overall_1"}
	cleanupCostAndOutlineEvents(t, eventIDs, outlineIDs)
	defer cleanupCostAndOutlineEvents(t, eventIDs, outlineIDs)

	db.Exec("DELETE FROM cost_events WHERE story_id = ?", storyID)
	defer db.Exec("DELETE FROM cost_events WHERE story_id = ?", storyID)

	now := time.Now()
	db.Create(&model.CostEvent{
		EventID: eventIDs[0], OccurredAt: now, Purpose: "story", Outcome: "ok",
		Provider: "doubao", Model: "pro", CostYuan: 0.50,
		StoryID: &storyID, PriceVersion: "v-test",
	})
	db.Create(&model.CostEvent{
		EventID: eventIDs[1], OccurredAt: now, Purpose: "tts", Outcome: "ok",
		Provider: "minimax", CostYuan: 0.40,
		StoryID: &storyID, PriceVersion: "v-test",
	})
	db.Create(&model.OutlineEvent{
		OutlineID: outlineIDs[0], OutlineGroupID: "g_agg",
		UserID: 1, ChildIDHash: "h", Outcome: "accepted",
		OccurredAt: now,
	})

	agg := cost.NewAggregator(db)
	since := now.Add(-1 * time.Hour)
	until := now.Add(1 * time.Hour)
	s, err := agg.Overall(context.Background(), since, until)
	require.NoError(t, err)

	if s.TotalYuan < 0.9 {
		t.Errorf("TotalYuan should include our 0.9 yuan, got %.4f", s.TotalYuan)
	}
	if s.StoriesAccepted < 1 {
		t.Errorf("StoriesAccepted should be >=1, got %d", s.StoriesAccepted)
	}
	if s.OutlinesAccepted < 1 {
		t.Errorf("OutlinesAccepted should be >=1, got %d", s.OutlinesAccepted)
	}
}

func TestAggregator_OutlineSaving_Formula(t *testing.T) {
	db := integrationDB(t)
	storyID := int64(99002)
	rejectedOutlineID := "ol_agg_saving_rejected"
	acceptedStoryEventIDs := []string{
		"agg22222:story:llm_call:1",
		"agg22222:tts:synthesize:1",
		"agg22222:storage_put:upload:1",
	}
	rejectedOutlineEventID := "agg33333:outline:llm_call:1"

	cleanupCostAndOutlineEvents(t,
		append(acceptedStoryEventIDs, rejectedOutlineEventID),
		[]string{rejectedOutlineID})
	defer cleanupCostAndOutlineEvents(t,
		append(acceptedStoryEventIDs, rejectedOutlineEventID),
		[]string{rejectedOutlineID})
	db.Exec("DELETE FROM cost_events WHERE story_id = ?", storyID)
	defer db.Exec("DELETE FROM cost_events WHERE story_id = ?", storyID)

	now := time.Now()
	for _, p := range []struct {
		evt, purpose, prov string
		yuan               float64
	}{
		{acceptedStoryEventIDs[0], "story", "doubao", 0.50},
		{acceptedStoryEventIDs[1], "tts", "minimax", 0.40},
		{acceptedStoryEventIDs[2], "storage_put", "tencent_cos", 0.10},
	} {
		db.Create(&model.CostEvent{
			EventID: p.evt, OccurredAt: now, Purpose: p.purpose, Outcome: "ok",
			Provider: p.prov, CostYuan: p.yuan, StoryID: &storyID, PriceVersion: "v-test",
		})
	}

	db.Create(&model.OutlineEvent{
		OutlineID: rejectedOutlineID, OutlineGroupID: "g_agg_rejected",
		UserID: 1, ChildIDHash: "h", Outcome: "refreshed",
		OccurredAt: now,
	})
	db.Create(&model.CostEvent{
		EventID: rejectedOutlineEventID, OccurredAt: now,
		Purpose: "outline", Outcome: "ok",
		Provider: "doubao", Model: "lite", CostYuan: 0.05,
		OutlineID: rejectedOutlineID, PriceVersion: "v-test",
	})

	agg := cost.NewAggregator(db)
	since := now.Add(-1 * time.Hour)
	until := now.Add(1 * time.Hour)
	saved, err := agg.OutlineSaving(context.Background(), since, until)
	require.NoError(t, err)

	// Expected = 1 × ~1.00 - 0.05 = ~0.95.
	// Loose bounds because AVG can be perturbed by other tests in the same window.
	if saved < 0.5 || saved > 2.0 {
		t.Errorf("OutlineSaving in plausible range: want [0.5, 2.0], got %.4f", saved)
	}
}
