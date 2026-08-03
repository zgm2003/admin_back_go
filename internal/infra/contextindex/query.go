package contextindex

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/google/uuid"
)

type QueryModality string

const (
	QueryModalityDense  QueryModality = "dense"
	QueryModalitySparse QueryModality = "sparse"
)

func (modality QueryModality) Valid() bool {
	return modality == QueryModalityDense || modality == QueryModalitySparse
}

type QueryVariantVector struct {
	VariantID   string
	QuerySHA256 [32]byte
	Dense       []float32
	Sparse      SparseVector
}

func (vector QueryVariantVector) Validate() error {
	if strings.TrimSpace(vector.VariantID) == "" || vector.QuerySHA256 == ([32]byte{}) {
		return errors.New("query variant identity must be present")
	}
	if err := validateDense(vector.Dense); err != nil {
		return err
	}
	return vector.Sparse.Validate()
}

type QueryBatchInput struct {
	Collection    string
	Filter        ScopeFilter
	Variants      []QueryVariantVector
	DenseMinScore *float64
	TopN          uint64
}

func (input QueryBatchInput) Validate() error {
	if err := ValidateCollectionName(strings.TrimSpace(input.Collection)); err != nil {
		return err
	}
	if err := input.Filter.Validate(); err != nil {
		return err
	}
	if len(input.Variants) == 0 || input.TopN == 0 || input.TopN > 200 {
		return errors.New("query batch requires variants and a top-n between 1 and 200")
	}
	if input.DenseMinScore != nil && (math.IsNaN(*input.DenseMinScore) || math.IsInf(*input.DenseMinScore, 0)) {
		return errors.New("dense minimum score must be finite")
	}
	seen := make(map[string]struct{}, len(input.Variants))
	for i, variant := range input.Variants {
		if err := variant.Validate(); err != nil {
			return fmt.Errorf("query variant %d: %w", i, err)
		}
		if _, duplicate := seen[variant.VariantID]; duplicate {
			return fmt.Errorf("duplicate query variant %q", variant.VariantID)
		}
		seen[variant.VariantID] = struct{}{}
	}
	return nil
}

type QueryFusionHit struct {
	Point PointRef
	Rank  uint64
	Score float64
}

type QueryBranchHit struct {
	Point     PointRef
	VariantID string
	Modality  QueryModality
	Rank      uint64
	Score     float64
}

type QueryBatchResult struct {
	Fusion   []QueryFusionHit
	Branches []QueryBranchHit
}

func (result QueryBatchResult) Validate() error {
	branchIDs := make(map[uuid.UUID]struct{}, len(result.Branches))
	for i, hit := range result.Branches {
		if err := validateQueryHit(hit.Point, hit.Rank, hit.Score); err != nil {
			return fmt.Errorf("branch hit %d: %w", i, err)
		}
		if strings.TrimSpace(hit.VariantID) == "" || !hit.Modality.Valid() {
			return errors.New("branch hit must have a closed variant and modality")
		}
		branchIDs[hit.Point.ID] = struct{}{}
	}
	for i, hit := range result.Fusion {
		if err := validateQueryHit(hit.Point, hit.Rank, hit.Score); err != nil {
			return fmt.Errorf("fusion hit %d: %w", i, err)
		}
		if _, ok := branchIDs[hit.Point.ID]; !ok {
			return fmt.Errorf("fusion point %s is absent from every branch", hit.Point.ID)
		}
	}
	return nil
}

func validateQueryHit(point PointRef, rank uint64, score float64) error {
	if err := point.Validate(); err != nil {
		return err
	}
	if rank == 0 || math.IsNaN(score) || math.IsInf(score, 0) {
		return errors.New("query hit rank and score must be finite")
	}
	return nil
}

type Querier interface {
	QueryBatch(context.Context, QueryBatchInput) (QueryBatchResult, error)
}
