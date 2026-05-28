package admin

import realtimemodule "admin_back_go/internal/module/realtime"

const (
	TypeConnectedV1  = realtimemodule.TypeConnectedV1
	TypePingV1       = realtimemodule.TypePingV1
	TypePongV1       = realtimemodule.TypePongV1
	TypeSubscribeV1  = realtimemodule.TypeSubscribeV1
	TypeSubscribedV1 = realtimemodule.TypeSubscribedV1
	TypeErrorV1      = realtimemodule.TypeErrorV1
)

type (
	Service = realtimemodule.Service
)

var NewService = realtimemodule.NewService
