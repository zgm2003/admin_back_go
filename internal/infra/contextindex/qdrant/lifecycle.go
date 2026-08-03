package qdrant

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"admin_back_go/internal/infra/contextindex"

	qdrantapi "github.com/qdrant/go-client/qdrant"
)

type lifecycleAPI interface {
	CollectionExists(context.Context, string) (bool, error)
	GetCollectionInfo(context.Context, string) (*qdrantapi.CollectionInfo, error)
	ListAliases(context.Context) ([]*qdrantapi.AliasDescription, error)
	UpdateAliases(context.Context, []*qdrantapi.AliasOperations) error
	Delete(context.Context, *qdrantapi.DeletePoints) (*qdrantapi.UpdateResult, error)
}

func (client *Client) EnsureCollection(ctx context.Context, spec contextindex.CollectionSpec) error {
	if client == nil || client.api == nil {
		return errors.New("Qdrant client is unavailable")
	}
	if err := spec.Validate(); err != nil {
		return err
	}
	api, ok := client.api.(lifecycleAPI)
	if !ok {
		return errors.New("Qdrant collection lifecycle protocol is unavailable")
	}
	exists, err := api.CollectionExists(ctx, spec.Name)
	if err != nil {
		return fmt.Errorf("inspect Qdrant collection %q: %w", spec.Name, err)
	}
	if !exists {
		return client.CreateCollection(ctx, spec)
	}
	active := contextindex.ActiveCollection{ProfileID: 1, IndexGeneration: 1, DenseDimensions: spec.DenseDimensions, DenseDistance: spec.DenseDistance}
	info, err := api.GetCollectionInfo(ctx, spec.Name)
	if err != nil {
		return err
	}
	return validateCollectionSchema(info, active)
}

func (client *Client) VerifyCollection(ctx context.Context, collection string, active contextindex.ActiveCollection) error {
	if err := contextindex.ValidateCollectionName(collection); err != nil {
		return err
	}
	if err := active.Validate(); err != nil {
		return err
	}
	api, ok := client.api.(lifecycleAPI)
	if !ok {
		return errors.New("Qdrant collection lifecycle protocol is unavailable")
	}
	info, err := api.GetCollectionInfo(ctx, collection)
	if err != nil {
		return fmt.Errorf("inspect Qdrant collection %q: %w", collection, err)
	}
	return validateReadyCollection(info, active)
}

func (client *Client) AliasTarget(ctx context.Context, alias string) (string, bool, error) {
	if err := contextindex.ValidateCollectionName(alias); err != nil {
		return "", false, err
	}
	api, ok := client.api.(lifecycleAPI)
	if !ok {
		return "", false, errors.New("Qdrant alias lifecycle protocol is unavailable")
	}
	aliases, err := api.ListAliases(ctx)
	if err != nil {
		return "", false, err
	}
	for _, current := range aliases {
		if current != nil && current.GetAliasName() == alias {
			return current.GetCollectionName(), true, nil
		}
	}
	return "", false, nil
}

func (client *Client) SwitchAlias(ctx context.Context, alias, collection string) error {
	if err := contextindex.ValidateCollectionName(alias); err != nil {
		return err
	}
	if err := contextindex.ValidateCollectionName(collection); err != nil {
		return err
	}
	api, ok := client.api.(lifecycleAPI)
	if !ok {
		return errors.New("Qdrant alias lifecycle protocol is unavailable")
	}
	current, exists, err := client.AliasTarget(ctx, alias)
	if err != nil || (exists && current == collection) {
		return err
	}
	actions := make([]*qdrantapi.AliasOperations, 0, 2)
	if exists {
		actions = append(actions, qdrantapi.NewAliasDelete(alias))
	}
	actions = append(actions, qdrantapi.NewAliasCreate(alias, collection))
	return api.UpdateAliases(ctx, actions)
}

func (client *Client) DeleteDocumentVersionPoints(ctx context.Context, collection string, profileID, generation, versionID uint64) error {
	if err := contextindex.ValidateCollectionName(collection); err != nil {
		return err
	}
	if profileID == 0 || generation == 0 || versionID == 0 {
		return errors.New("document point cleanup identity is incomplete")
	}
	api, ok := client.api.(lifecycleAPI)
	if !ok {
		return errors.New("Qdrant point cleanup protocol is unavailable")
	}
	wait := true
	_, err := api.Delete(ctx, &qdrantapi.DeletePoints{CollectionName: collection, Wait: &wait,
		Points: qdrantapi.NewPointsSelectorFilter(&qdrantapi.Filter{Must: []*qdrantapi.Condition{
			qdrantapi.NewMatchInt("profile_id", int64(profileID)),
			qdrantapi.NewMatchInt("index_generation", int64(generation)),
			qdrantapi.NewMatchInt("document_version_id", int64(versionID)),
			qdrantapi.NewMatch("source_kind", string(contextindex.SourceKindDocumentChunk)),
		}})})
	return err
}

func (client *Client) DeleteConversationTurnPoint(ctx context.Context, collection string, profileID, generation, userMessageID uint64, sourceSHA256 [32]byte) error {
	if err := contextindex.ValidateCollectionName(collection); err != nil {
		return err
	}
	if profileID == 0 || generation == 0 || userMessageID == 0 || sourceSHA256 == ([32]byte{}) {
		return errors.New("conversation point cleanup identity is incomplete")
	}
	api, ok := client.api.(lifecycleAPI)
	if !ok {
		return errors.New("Qdrant point cleanup protocol is unavailable")
	}
	wait := true
	_, err := api.Delete(ctx, &qdrantapi.DeletePoints{CollectionName: collection, Wait: &wait,
		Points: qdrantapi.NewPointsSelectorFilter(&qdrantapi.Filter{Must: []*qdrantapi.Condition{
			qdrantapi.NewMatchInt("profile_id", int64(profileID)),
			qdrantapi.NewMatchInt("index_generation", int64(generation)),
			qdrantapi.NewMatchInt("source_id", int64(userMessageID)),
			qdrantapi.NewMatch("source_kind", string(contextindex.SourceKindConversationTurn)),
			qdrantapi.NewMatch("source_sha256", hex.EncodeToString(sourceSHA256[:])),
		}})})
	return err
}

func validateCollectionSchema(info *qdrantapi.CollectionInfo, active contextindex.ActiveCollection) error {
	if info == nil {
		return errors.New("collection info is missing")
	}
	copy := *info
	copy.Status = qdrantapi.CollectionStatus_Green
	return validateReadyCollection(&copy, active)
}
