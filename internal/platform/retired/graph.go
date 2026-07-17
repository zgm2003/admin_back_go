package retired

import (
	aiassetcanvas "admin_back_go/internal/module/ai/asset/transport/canvas"
	aiaudio "admin_back_go/internal/module/ai/audio"
	aichat "admin_back_go/internal/module/ai/chat"
	aiimage "admin_back_go/internal/module/ai/image"
	aipromptcanvas "admin_back_go/internal/module/ai/prompt/transport/canvas"
	aivideo "admin_back_go/internal/module/ai/video"
	canvastransport "admin_back_go/internal/module/canvas/transport/canvas"
)

// Graph is a temporary typed holder for transports retired by P09. Admin
// contract generation must never consume this graph.
type Graph struct {
	Canvas   canvastransport.HTTPService
	AIAssets aiassetcanvas.HTTPService
	AIAudio  aiaudio.HTTPService
	AIChat   aichat.HTTPService
	AIImages aiimage.HTTPService
	AIPrompt aipromptcanvas.HTTPService
	AIVideo  aivideo.HTTPService
}
