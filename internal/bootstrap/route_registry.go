package bootstrap

import "admin_back_go/internal/server/adminroute"

// AdminRouteRegistry starts empty. Transport packages register each Gin route
// together with its access and audit policy during router construction.
func AdminRouteRegistry() (*adminroute.Registry, error) {
	return adminroute.NewRegistry(), nil
}
