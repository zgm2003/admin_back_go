package databaseevolution

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"admin_back_go/internal/infra/secretbox"
	"admin_back_go/internal/infra/secretkey"
	storagecos "admin_back_go/internal/infra/storage/cos"
)

const (
	COSReferenceReachable  = "reachable"
	COSReferenceNotFound   = "not_found"
	COSReferenceDependency = "dependency"
)

type COSReferenceResult struct {
	Key             string `json:"key"`
	Status          string `json:"status"`
	DependencyClass string `json:"dependency_class,omitempty"`
}

func VerifyCOSReferences(ctx context.Context, reader storagecos.ObjectReader, input storagecos.GetInput, keys []string) []COSReferenceResult {
	normalized := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		key = strings.TrimLeft(strings.TrimSpace(key), "/")
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, key)
	}
	sort.Strings(normalized)

	results := make([]COSReferenceResult, len(normalized))
	type referenceJob struct {
		index int
		key   string
	}
	jobs := make(chan referenceJob)
	workerCount := 8
	if len(normalized) < workerCount {
		workerCount = len(normalized)
	}
	var workers sync.WaitGroup
	workers.Add(workerCount)
	for range workerCount {
		go func() {
			defer workers.Done()
			for job := range jobs {
				results[job.index] = verifyCOSReference(ctx, reader, input, job.key)
			}
		}()
	}
	for index, key := range normalized {
		jobs <- referenceJob{index: index, key: key}
	}
	close(jobs)
	workers.Wait()
	return results
}

func verifyCOSReference(ctx context.Context, reader storagecos.ObjectReader, input storagecos.GetInput, key string) COSReferenceResult {
	request := input
	request.Key = key
	request.Range = "bytes=0-0"
	_, err := reader.Get(ctx, request)
	result := COSReferenceResult{Key: key, Status: COSReferenceReachable}
	switch {
	case err == nil:
	case storagecos.IsNotFound(err):
		result.Status = COSReferenceNotFound
	default:
		result.Status = COSReferenceDependency
		result.DependencyClass = cosDependencyClass(err)
	}
	return result
}

func VerifyStoredCOSReferences(ctx context.Context, database *sql.DB, rootSecret string) ([]COSReferenceResult, error) {
	if database == nil {
		return nil, fmt.Errorf("database connection is required")
	}
	keys, err := loadCOSReferenceKeys(ctx, database)
	if err != nil {
		return nil, err
	}
	config, err := loadCOSReferenceConfig(ctx, database, rootSecret)
	if err != nil {
		return nil, err
	}
	reader := storagecos.NewObjectReader(storagecos.ObjectReaderConfig{
		Enabled:  true,
		Timeout:  15 * time.Second,
		MaxBytes: 1,
	})
	return VerifyCOSReferences(ctx, reader, config, keys), nil
}

func loadCOSReferenceKeys(ctx context.Context, database *sql.DB) ([]string, error) {
	rows, err := database.QueryContext(ctx, `
SELECT DISTINCT storage_key
FROM ai_image_files
WHERE storage_key IS NOT NULL AND storage_key<>''
ORDER BY storage_key`)
	if err != nil {
		return nil, fmt.Errorf("load COS reference keys: %w", err)
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scan COS reference key: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate COS reference keys: %w", err)
	}
	return keys, nil
}

func loadCOSReferenceConfig(ctx context.Context, database *sql.DB, rootSecret string) (storagecos.GetInput, error) {
	var secretIDCiphertext string
	var secretKeyCiphertext string
	var input storagecos.GetInput
	err := database.QueryRowContext(ctx, `
SELECT d.secret_id_enc, d.secret_key_enc, d.bucket, d.region, d.endpoint
FROM upload_setting s
JOIN upload_driver d ON d.id=s.driver_id AND d.is_del=2 AND d.driver='cos'
JOIN upload_rule r ON r.id=s.rule_id AND r.is_del=2
WHERE s.status=1 AND s.is_del=2
ORDER BY s.id DESC
LIMIT 1`).Scan(&secretIDCiphertext, &secretKeyCiphertext, &input.Bucket, &input.Region, &input.Endpoint)
	if err != nil {
		return storagecos.GetInput{}, fmt.Errorf("load enabled COS configuration: %w", err)
	}
	keys, err := secretkey.NewKeyRing(rootSecret)
	if err != nil {
		return storagecos.GetInput{}, fmt.Errorf("derive COS credential key: %w", err)
	}
	box := secretbox.New(keys.SecretboxKey())
	input.SecretID, err = box.Decrypt(secretIDCiphertext)
	if err != nil {
		return storagecos.GetInput{}, fmt.Errorf("decrypt COS secret id: %w", err)
	}
	input.SecretKey, err = box.Decrypt(secretKeyCiphertext)
	if err != nil {
		return storagecos.GetInput{}, fmt.Errorf("decrypt COS secret key: %w", err)
	}
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	if input.Endpoint != "" && !strings.Contains(input.Endpoint, "://") {
		input.Endpoint = "https://" + input.Endpoint
	}
	return input, nil
}

func WriteCOSReferenceManifest(outputPath string, results []COSReferenceResult) error {
	outputPath = strings.TrimSpace(outputPath)
	if outputPath == "" {
		return fmt.Errorf("output path is required")
	}
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("encode COS reference manifest: %w", err)
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(outputPath), "."+filepath.Base(outputPath)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary COS reference manifest: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("secure COS reference manifest: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		return fmt.Errorf("write COS reference manifest: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync COS reference manifest: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close COS reference manifest: %w", err)
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("replace COS reference manifest: %w", err)
	}
	committed = true
	return nil
}

func cosDependencyClass(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if status, ok := storagecos.HTTPStatus(err); ok {
		switch {
		case status == 401 || status == 403:
			return "auth"
		case status == 429:
			return "throttled"
		case status >= 500:
			return "provider"
		default:
			return "http"
		}
	}
	var urlError *url.Error
	if errors.As(err, &urlError) {
		return "network"
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return "network"
	}
	return "dependency"
}
