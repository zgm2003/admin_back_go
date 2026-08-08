package storage

import (
	"context"
	"errors"
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

func TestConditionalObjectPreviewContractValidatesPersistedFacts(t *testing.T) {
	var previewer ConditionalObjectPreviewer = conditionalObjectPreviewStub{}
	result, err := previewer.Preview(context.Background(), ConditionalObjectPreviewInput{
		Object:   ConditionalObjectInput{StorageProvider: "cos", ObjectKey: "ai_context_documents/report.md", ETag: `"v1"`, Size: 4},
		MIMEType: "text/markdown",
	})
	if err != nil || result.URL == "" || result.ExpiresIn != 300 || result.Metadata.MIMEType != "text/markdown" {
		t.Fatalf("preview=%#v err=%v", result, err)
	}
	if err := (ConditionalObjectPreviewInput{Object: ConditionalObjectInput{StorageProvider: "cos", ObjectKey: "ai_context_documents/report.md", ETag: `"v1"`, Size: 4}, MIMEType: ""}).Validate(); !errors.Is(err, ErrInvalidConditionalObjectPreview) {
		t.Fatalf("invalid MIME error=%v", err)
	}
}

type conditionalObjectStub struct{}

func (conditionalObjectStub) Head(context.Context, ConditionalObjectInput) (ConditionalObjectMetadata, error) {
	return ConditionalObjectMetadata{ETag: `"v1"`, Size: 4, MIMEType: "application/pdf"}, nil
}
func (conditionalObjectStub) Open(context.Context, ConditionalObjectInput) (io.ReadCloser, ConditionalObjectMetadata, error) {
	return nil, ConditionalObjectMetadata{}, nil
}

type conditionalObjectPreviewStub struct{}

func (conditionalObjectPreviewStub) Preview(context.Context, ConditionalObjectPreviewInput) (ConditionalObjectPreview, error) {
	return ConditionalObjectPreview{URL: "https://cos.example/report.md?signature=secret", ExpiresIn: 300, Metadata: ConditionalObjectMetadata{ETag: `"v1"`, Size: 4, MIMEType: "text/markdown"}}, nil
}
