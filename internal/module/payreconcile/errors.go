package payreconcile

import "errors"

var ErrRepositoryNotConfigured = errors.New("pay reconcile repository is not configured")

var ErrPlatformBillDownloadNotImplemented = errors.New("pay reconcile platform bill download is not implemented")
