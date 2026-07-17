package user

import "admin_back_go/internal/module/permission"

var _ permission.PrincipalVersionBumper = (*GormRepository)(nil)
