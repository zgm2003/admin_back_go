package aimessage

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"gorm.io/gorm"
)

var messageCitationPattern = regexp.MustCompile(`\[C([1-9][0-9]*)\]`)

type MessageContext struct {
	PlanID      uint64           `json:"plan_id"`
	Outcome     string           `json:"outcome"`
	Sources     []CitationSource `json:"sources"`
	InvalidKeys []string         `json:"invalid_keys"`
}

type CitationSource struct {
	Key               string          `json:"key"`
	Cited             bool            `json:"cited"`
	Title             string          `json:"title"`
	Locator           CitationLocator `json:"locator"`
	DocumentID        uint64          `json:"document_id"`
	DocumentVersionID uint64          `json:"document_version_id"`
}

type CitationLocator struct {
	Schema      string   `json:"schema"`
	Kind        string   `json:"kind"`
	Page        *uint32  `json:"page,omitempty"`
	Paragraph   *uint32  `json:"paragraph,omitempty"`
	LineStart   *uint32  `json:"line_start,omitempty"`
	LineEnd     *uint32  `json:"line_end,omitempty"`
	RowStart    *uint32  `json:"row_start,omitempty"`
	RowEnd      *uint32  `json:"row_end,omitempty"`
	Sheet       *string  `json:"sheet,omitempty"`
	CellStart   *string  `json:"cell_start,omitempty"`
	CellEnd     *string  `json:"cell_end,omitempty"`
	HeadingPath []string `json:"heading_path,omitempty"`
}

type messageContextPlan struct {
	ID               uint64
	RunID            uint64
	RetrievalOutcome string
	State            string
	Items            []messageContextPlanItem
}

type messageContextPlanItem struct {
	Decision     string
	Kind         string
	CitationKey  *string
	MetadataJSON string
}

type messageContextMetadata struct {
	Document *messageContextDocument `json:"document"`
}

type messageContextDocument struct {
	Title             string            `json:"title"`
	DocumentID        uint64            `json:"document_id"`
	DocumentVersionID uint64            `json:"document_version_id"`
	Locators          []CitationLocator `json:"locators"`
}

type ContextPlanRepository interface {
	ContextPlans(context.Context, []uint64) (map[uint64]messageContextPlan, error)
}

func projectMessageContext(content string, plan messageContextPlan) (MessageContext, error) {
	if plan.ID == 0 || plan.RunID == 0 || plan.State != "ready" {
		return MessageContext{}, errors.New("invalid message context plan")
	}
	result := MessageContext{
		PlanID:      plan.ID,
		Outcome:     plan.RetrievalOutcome,
		Sources:     make([]CitationSource, 0),
		InvalidKeys: make([]string, 0),
	}
	sourceIndexes := make(map[string]int)
	for _, item := range plan.Items {
		if item.Decision != "selected" || item.Kind != "document_evidence" || item.CitationKey == nil {
			continue
		}
		var metadata messageContextMetadata
		if json.Unmarshal([]byte(item.MetadataJSON), &metadata) != nil || metadata.Document == nil || metadata.Document.DocumentID == 0 ||
			metadata.Document.DocumentVersionID == 0 || metadata.Document.Title == "" || len(metadata.Document.Locators) == 0 {
			return MessageContext{}, errors.New("invalid message context evidence")
		}
		key := strings.TrimSpace(*item.CitationKey)
		if key == "" {
			return MessageContext{}, errors.New("invalid message context citation")
		}
		sourceIndexes[key] = len(result.Sources)
		result.Sources = append(result.Sources, CitationSource{Key: key, Title: metadata.Document.Title,
			Locator: metadata.Document.Locators[0], DocumentID: metadata.Document.DocumentID, DocumentVersionID: metadata.Document.DocumentVersionID})
	}
	seenInvalid := make(map[string]struct{})
	for _, match := range messageCitationPattern.FindAllStringSubmatch(content, -1) {
		key := "C" + match[1]
		if index, ok := sourceIndexes[key]; ok {
			result.Sources[index].Cited = true
			continue
		}
		if _, exists := seenInvalid[key]; exists {
			continue
		}
		seenInvalid[key] = struct{}{}
		result.InvalidKeys = append(result.InvalidKeys, key)
	}
	return result, nil
}

type messageContextPlanRow struct {
	ID               uint64 `gorm:"column:id"`
	RunID            uint64 `gorm:"column:run_id"`
	RetrievalOutcome string `gorm:"column:retrieval_outcome"`
	State            string `gorm:"column:state"`
}

type messageContextPlanItemRow struct {
	PlanID       uint64  `gorm:"column:plan_id"`
	Decision     string  `gorm:"column:decision"`
	Kind         string  `gorm:"column:block_kind"`
	CitationKey  *string `gorm:"column:citation_key"`
	MetadataJSON string  `gorm:"column:metadata_json"`
}

func loadMessageContextPlans(ctx context.Context, db *gorm.DB, runIDs []uint64) (map[uint64]messageContextPlan, error) {
	if db == nil || len(runIDs) == 0 {
		return map[uint64]messageContextPlan{}, nil
	}
	var rows []messageContextPlanRow
	if err := db.WithContext(ctx).Table("ai_context_plans").Where("run_id IN ? AND state = ?", runIDs, "ready").Find(&rows).Error; err != nil {
		return nil, err
	}
	plans := make(map[uint64]messageContextPlan, len(rows))
	planIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		if _, exists := plans[row.RunID]; exists {
			return nil, errors.New("multiple ready context plans for one run")
		}
		plans[row.RunID] = messageContextPlan{ID: row.ID, RunID: row.RunID, RetrievalOutcome: row.RetrievalOutcome, State: row.State}
		planIDs = append(planIDs, row.ID)
	}
	if len(planIDs) == 0 {
		return plans, nil
	}
	var itemRows []messageContextPlanItemRow
	if err := db.WithContext(ctx).Table("ai_context_plan_items").Where("plan_id IN ?", planIDs).Order("plan_id ASC, ordinal ASC").Find(&itemRows).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint64][]messageContextPlanItem, len(planIDs))
	for _, row := range itemRows {
		byID[row.PlanID] = append(byID[row.PlanID], messageContextPlanItem{Decision: row.Decision, Kind: row.Kind, CitationKey: row.CitationKey, MetadataJSON: row.MetadataJSON})
	}
	for runID, plan := range plans {
		plan.Items = byID[plan.ID]
		plans[runID] = plan
	}
	return plans, nil
}
