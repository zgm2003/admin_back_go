package contextindex

import (
	"errors"
	"fmt"
)

const maxDenseDimensions = 65_536

type ActiveCollection struct {
	ProfileID       uint64
	IndexGeneration uint64
	DenseDimensions uint64
	DenseDistance   Distance
}

func (collection ActiveCollection) Validate() error {
	if !validQdrantID(collection.ProfileID) {
		return errors.New("active collection profile ID must fit a positive Qdrant integer")
	}
	if !validQdrantID(collection.IndexGeneration) {
		return errors.New("active collection generation must fit a positive Qdrant integer")
	}
	if collection.DenseDimensions == 0 || collection.DenseDimensions > maxDenseDimensions {
		return fmt.Errorf("active collection dense dimensions must be between 1 and %d", maxDenseDimensions)
	}
	if !collection.DenseDistance.Valid() {
		return fmt.Errorf("unsupported active collection distance %q", collection.DenseDistance)
	}
	return nil
}
