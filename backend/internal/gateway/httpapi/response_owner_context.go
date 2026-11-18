package httpapi

import "github.com/gin-gonic/gin"

const responseOwnerContextKey = "openai_http_response_owner"

// HTTPResponseOwner 记录成功续接绑定的下游主体。
type HTTPResponseOwner struct{ UserID, APIKeyID int64 }

// SetHTTPResponseOwner 将认证入口提供的有效标识写入请求上下文。
func SetHTTPResponseOwner(c *gin.Context, userID, keyID int64) {
	if c == nil || userID <= 0 || keyID <= 0 {
		return
	}
	c.Set(responseOwnerContextKey, HTTPResponseOwner{UserID: userID, APIKeyID: keyID})
}

// ResponseOwnerFromContext 从认证后的请求上下文读取归属。
func ResponseOwnerFromContext(c *gin.Context) (HTTPResponseOwner, bool) {
	value, ok := c.Get(responseOwnerContextKey)
	if !ok {
		return HTTPResponseOwner{}, false
	}
	owner, ok := value.(HTTPResponseOwner)
	return owner, ok && owner.UserID > 0 && owner.APIKeyID > 0
}
