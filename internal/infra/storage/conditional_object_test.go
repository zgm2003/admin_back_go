package storage

import (
	"context"
	"io"
	"testing"
)

func TestConditionalObjectContractIsBusinessNeutral(t *testing.T) {
	var reader ConditionalObjectReader = conditionalObjectStub{}
	metadata, err := reader.Head(context.Background(), ConditionalObjectInput{
		StorageProvider: "cos", ObjectKey: "objects/report.pdf", ETag: `"v1"`, Size: 4,
	})
	if err != nil || metadata.ETag != `"v1"` || metadata.Size != 4 {
		t.Fatalf("metadata=%#v err=%v", metadata, err)
	}
}

type conditionalObjectStub struct{}

func (conditionalObjectStub) Head(context.Context, ConditionalObjectInput) (ConditionalObjectMetadata, error) {
	return ConditionalObjectMetadata{ETag: `"v1"`, Size: 4, MIMEType: "application/pdf"}, nil
}
func (conditionalObjectStub) Open(context.Context, ConditionalObjectInput) (io.ReadCloser, ConditionalObjectMetadata, error) {
	return nil, ConditionalObjectMetadata{}, nil
}
