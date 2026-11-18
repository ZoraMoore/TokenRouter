package httpapi

import (
	"github.com/gin-gonic/gin"
)

// RegisterRedeemCodeRoutes 注册所属管理路由；组鉴权、限流和审计由 app 预先安装。
func RegisterRedeemCodeRoutes(admin *gin.RouterGroup, endpoint *AdminRedeemHandler) {
	codes := admin.Group("/redeem-codes")
	{
		codes.GET("", endpoint.List)
		codes.GET("/export", endpoint.Export)
		codes.GET("/:id", endpoint.GetByID)
		codes.POST("/create-and-redeem", endpoint.CreateAndRedeem)
		codes.POST("/generate", endpoint.Generate)
		codes.PUT("/:id", endpoint.Update)
		codes.DELETE("/:id", endpoint.Delete)
		codes.POST("/batch-delete", endpoint.BatchDelete)
		codes.POST("/batch-update", endpoint.BatchUpdate)
		codes.POST("/:id/expire", endpoint.Expire)
	}
}

// RegisterSubscriptionRoutes 注册所属管理路由；组鉴权、限流和审计由 app 预先安装。
func RegisterSubscriptionRoutes(admin *gin.RouterGroup, endpoint *AdminSubscriptionHandler) {
	subscriptions := admin.Group("/subscriptions")
	{
		subscriptions.GET("", endpoint.List)
		subscriptions.GET("/:id", endpoint.GetByID)
		subscriptions.GET("/:id/progress", endpoint.GetProgress)
		subscriptions.POST("/assign", endpoint.Assign)
		subscriptions.POST("/bulk-assign", endpoint.BulkAssign)
		subscriptions.POST("/:id/extend", endpoint.Extend)
		subscriptions.POST("/:id/reset-quota", endpoint.ResetQuota)
		subscriptions.POST("/:id/revoke", endpoint.Revoke)
		subscriptions.POST("/:id/restore", endpoint.Restore)
		subscriptions.DELETE("/:id", endpoint.Revoke)
	}

	// 套餐下的订阅列表
	admin.GET("/plans/:id/subscriptions", endpoint.ListByPlan)

	// 用户下的订阅列表
	admin.GET("/users/:id/subscriptions", endpoint.ListByUser)
}
