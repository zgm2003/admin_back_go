package middleware

import (
	"admin_back_go/internal/config"

	gincors "github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS(cfg config.CORSConfig) gin.HandlerFunc {
	if isZeroCORSConfig(cfg) {
		cfg = config.DefaultCORSConfig()
	}

	return gincors.New(gincors.Config{
		AllowOrigins:     cfg.AllowOrigins,
		AllowMethods:     cfg.AllowMethods,
		AllowHeaders:     cfg.AllowHeaders,
		ExposeHeaders:    cfg.ExposeHeaders,
		AllowCredentials: cfg.AllowCredentials,
		MaxAge:           cfg.MaxAge,
	})
}

func isZeroCORSConfig(cfg config.CORSConfig) bool {
	return len(cfg.AllowOrigins) == 0 &&
		len(cfg.AllowMethods) == 0 &&
		len(cfg.AllowHeaders) == 0 &&
		len(cfg.ExposeHeaders) == 0 &&
		!cfg.AllowCredentials &&
		cfg.MaxAge == 0
}
