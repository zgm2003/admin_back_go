package qdrant

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"admin_back_go/internal/infra/contextindex"

	qdrantapi "github.com/qdrant/go-client/qdrant"
)

type readinessAPI interface {
	HealthCheck(context.Context) (*qdrantapi.HealthCheckReply, error)
	GetCollectionInfo(context.Context, string) (*qdrantapi.CollectionInfo, error)
	ListAliases(context.Context) ([]*qdrantapi.AliasDescription, error)
	QueryBatch(context.Context, *qdrantapi.QueryBatchPoints) ([]*qdrantapi.BatchResult, error)
}

func (client *Client) CheckReadiness(ctx context.Context, collectionPrefix string, active []contextindex.ActiveCollection) error {
	if client == nil || client.api == nil {
		return errors.New("Qdrant client is unavailable")
	}
	api, ok := client.api.(readinessAPI)
	if !ok {
		return errors.New("Qdrant readiness protocol is unavailable")
	}
	if err := contextindex.ValidateCollectionName(collectionPrefix); err != nil {
		return fmt.Errorf("invalid Qdrant collection prefix: %w", err)
	}
	health, err := api.HealthCheck(ctx)
	if err != nil {
		return errors.New("Qdrant health check failed")
	}
	if err := requireServerVersion(health.GetVersion()); err != nil {
		return err
	}
	aliases, err := api.ListAliases(ctx)
	if err != nil {
		return errors.New("Qdrant alias inspection failed")
	}
	aliasTargets := make(map[string]string, len(aliases))
	for _, alias := range aliases {
		if alias == nil || alias.GetAliasName() == "" || alias.GetCollectionName() == "" {
			return errors.New("Qdrant returned an invalid alias")
		}
		if _, exists := aliasTargets[alias.GetAliasName()]; exists {
			return errors.New("Qdrant returned a duplicate alias")
		}
		aliasTargets[alias.GetAliasName()] = alias.GetCollectionName()
	}
	for _, collection := range active {
		if err := collection.Validate(); err != nil {
			return err
		}
		aliasName := fmt.Sprintf("%s_profile_%d", collectionPrefix, collection.ProfileID)
		physicalName := fmt.Sprintf("%s_g%d", aliasName, collection.IndexGeneration)
		if aliasTargets[aliasName] != physicalName {
			return fmt.Errorf("Qdrant alias %q disagrees with the active generation", aliasName)
		}
		info, err := api.GetCollectionInfo(ctx, physicalName)
		if err != nil {
			return fmt.Errorf("inspect Qdrant collection %q: %w", physicalName, err)
		}
		if err := validateReadyCollection(info, collection); err != nil {
			return fmt.Errorf("Qdrant collection %q: %w", physicalName, err)
		}
		if err := probeQueryBatchRRF(ctx, api, physicalName, collection); err != nil {
			return fmt.Errorf("Qdrant collection %q query capability: %w", physicalName, err)
		}
	}
	return nil
}

func requireServerVersion(raw string) error {
	version := strings.SplitN(strings.TrimPrefix(strings.TrimSpace(raw), "v"), "-", 2)[0]
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return errors.New("Qdrant server returned an invalid version")
	}
	numbers := make([]int, 3)
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return errors.New("Qdrant server returned an invalid version")
		}
		numbers[index] = value
	}
	if numbers[0] != 1 || numbers[1] != 18 || numbers[2] != 3 {
		return errors.New("Qdrant server does not satisfy the context query contract")
	}
	return nil
}

func validateReadyCollection(info *qdrantapi.CollectionInfo, active contextindex.ActiveCollection) error {
	if info == nil || info.GetStatus() != qdrantapi.CollectionStatus_Green {
		return errors.New("collection is not green")
	}
	params := info.GetConfig().GetParams()
	dense := params.GetVectorsConfig().GetParamsMap().GetMap()[denseVectorName]
	wantDistance, err := denseDistance(active.DenseDistance)
	if err != nil {
		return err
	}
	if dense == nil || dense.GetSize() != active.DenseDimensions || dense.GetDistance() != wantDistance {
		return errors.New("dense vector schema mismatch")
	}
	sparse := params.GetSparseVectorsConfig().GetMap()[sparseVectorName]
	if sparse == nil || sparse.GetModifier() != qdrantapi.Modifier_Idf {
		return errors.New("sparse IDF schema mismatch")
	}
	for _, field := range payloadIndexFields {
		schema := info.GetPayloadSchema()[field.name]
		if schema == nil || schema.GetDataType() != payloadSchemaType(field.fieldType) {
			return fmt.Errorf("payload index %q schema mismatch", field.name)
		}
	}
	return nil
}

func payloadSchemaType(fieldType qdrantapi.FieldType) qdrantapi.PayloadSchemaType {
	switch fieldType {
	case qdrantapi.FieldType_FieldTypeKeyword:
		return qdrantapi.PayloadSchemaType_Keyword
	case qdrantapi.FieldType_FieldTypeInteger:
		return qdrantapi.PayloadSchemaType_Integer
	default:
		return qdrantapi.PayloadSchemaType_UnknownType
	}
}

func probeQueryBatchRRF(ctx context.Context, api readinessAPI, collectionName string, active contextindex.ActiveCollection) error {
	limit := uint64(1)
	using := denseVectorName
	filter := &qdrantapi.Filter{Must: []*qdrantapi.Condition{
		qdrantapi.NewMatchInt("profile_id", int64(active.ProfileID)),
		qdrantapi.NewMatchInt("index_generation", int64(active.IndexGeneration)),
	}}
	branch := &qdrantapi.PrefetchQuery{
		Query:  qdrantapi.NewQueryDense(make([]float32, active.DenseDimensions)),
		Using:  &using,
		Filter: filter,
		Limit:  &limit,
	}
	sparseBranch := &qdrantapi.PrefetchQuery{
		Query:  qdrantapi.NewQuerySparse([]uint32{0}, []float32{1}),
		Using:  stringPointer(sparseVectorName),
		Filter: filter,
		Limit:  &limit,
	}
	k := rrfK
	results, err := api.QueryBatch(ctx, &qdrantapi.QueryBatchPoints{
		CollectionName: collectionName,
		QueryPoints: []*qdrantapi.QueryPoints{
			readinessBranchQuery(collectionName, branch),
			readinessBranchQuery(collectionName, sparseBranch),
			{
				CollectionName: collectionName,
				Prefetch:       []*qdrantapi.PrefetchQuery{branch, sparseBranch},
				Query:          qdrantapi.NewQueryRRF(&qdrantapi.Rrf{K: &k}),
				Filter:         filter,
				Limit:          &limit,
				WithPayload:    qdrantapi.NewWithPayload(false),
			},
		},
	})
	if err != nil {
		return errors.New("QueryBatch/RRF probe failed")
	}
	if len(results) != 3 {
		return fmt.Errorf("QueryBatch/RRF probe returned %d results, want 3", len(results))
	}
	return nil
}

func readinessBranchQuery(collectionName string, branch *qdrantapi.PrefetchQuery) *qdrantapi.QueryPoints {
	return &qdrantapi.QueryPoints{
		CollectionName: collectionName,
		Query:          branch.Query,
		Using:          branch.Using,
		Filter:         branch.Filter,
		Limit:          branch.Limit,
		WithPayload:    qdrantapi.NewWithPayload(false),
	}
}
