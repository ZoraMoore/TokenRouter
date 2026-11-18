package httpapi

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/egress/provider"
	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
)

const authenticatedIngressObservationMaxBodyBytes = 512 << 10

// RegisterAuthenticatedIngressTLSRoute 注册只供本机 observer 使用的内部入口。
// 公网经过 Nginx 时不会持有 observer token，未知或畸形载荷一律安全拒绝。
func RegisterAuthenticatedIngressTLSRoute(r *gin.Engine, ingress *provider.AuthenticatedIngressTLSService) {
	if r == nil {
		return
	}
	r.POST("/internal/tls-fingerprint/authenticated-ingress/observations", func(c *gin.Context) {
		if ingress == nil || !ingress.VerifyObserverToken(c.GetHeader("Authorization")) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		reader := http.MaxBytesReader(c.Writer, c.Request.Body, authenticatedIngressObservationMaxBodyBytes)
		defer func() { _ = reader.Close() }()
		decoder := json.NewDecoder(reader)
		decoder.DisallowUnknownFields()
		var observation tlsfingerprint.AuthenticatedIngressObservation
		if err := decoder.Decode(&observation); err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid observation"})
			return
		}
		var trailing any
		if err := decoder.Decode(&trailing); err != io.EOF {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid observation"})
			return
		}
		if err := ingress.AcceptObservation(&observation); err != nil {
			// 不回显内部解析原因，避免该入口成为 TLS 结构探针。
			c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"error": "observation rejected"})
			return
		}
		c.Status(http.StatusAccepted)
	})
}
