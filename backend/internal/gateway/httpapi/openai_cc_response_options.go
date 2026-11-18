package httpapi

import (
	"bufio"
	"io"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

func (p *OpenAIResponseOutput) CCOptions(c *gin.Context, writeError func(*gin.Context, int, string, string)) openai.CCResponseOptions {
	observer := UpstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = BeginUpstreamResponseModelObservation(c)
	}
	maxLineSize := OpenAIResponseDefaultMaxLineSize
	if p.Options.Configured && p.Options.MaxLineSize > 0 {
		maxLineSize = p.Options.MaxLineSize
	}
	return openai.CCResponseOptions{
		MaxLineSize: maxLineSize,
		ObserveChunk: func(body []byte) {
			observer.ObserveOpenAI(body, "")
		},
		ObserveJSON: func(body []byte) {
			observer.ObserveOpenAI(body, "response.completed")
		},
		ServiceTier: observer.ServiceTier,
		ReadBody: func(r io.Reader) ([]byte, error) {
			return ReadUpstreamResponseBody(r, p.Options.ReadLimit, c, OpenAIResponseTooLarge)
		},
		BodyLimitError: httpclient.ErrResponseBodyTooLarge,
		WriteError:     func(status int, kind, message string) { writeError(c, status, kind, message) },
	}
}

func (p *OpenAIResponseOutput) Scanner(r io.Reader) *bufio.Scanner {
	maxLineSize := OpenAIResponseDefaultMaxLineSize
	if p.Options.Configured && p.Options.MaxLineSize > 0 {
		maxLineSize = p.Options.MaxLineSize
	}
	return openai.NewCompatSSEScanner(r, maxLineSize)
}
