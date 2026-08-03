package contextengine

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"

	"admin_back_go/internal/infra/contextindex"

	"github.com/google/uuid"
)

func PointID(profileID uint64, sourceKind contextindex.SourceKind, sourceID uint64, sourceSHA256 [sha256.Size]byte) (uuid.UUID, error) {
	if profileID == 0 || profileID > math.MaxInt64 || sourceID == 0 || sourceID > math.MaxInt64 || !sourceKind.Valid() || sourceSHA256 == ([sha256.Size]byte{}) {
		return uuid.Nil, errors.New("point identity facts are invalid")
	}
	preimage := fmt.Sprintf("admin-context-point-v1\x00%d\x00%s\x00%d\x00%s", profileID, sourceKind, sourceID, hex.EncodeToString(sourceSHA256[:]))
	digest := sha256.Sum256([]byte(preimage))
	var id uuid.UUID
	copy(id[:], digest[:16])
	id[6] = id[6]&0x0f | 0x80
	id[8] = id[8]&0x3f | 0x80
	return id, nil
}
