package clientversion

import "errors"

var ErrRepositoryNotConfigured = errors.New("client version repository is not configured")
var ErrPublisherNotConfigured = errors.New("client version manifest publisher is not configured")
