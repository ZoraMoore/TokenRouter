package httpx

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/klauspost/compress/zstd"
)

const (
	requestBodyReadInitCap    = 512
	requestBodyReadMaxInitCap = 1 << 20
	maxDecompressedBodySize   = 64 << 20
)

type requestBodyReadKey struct{}

// RequestBodyRead 保存读取耗时、收到的字节数和解压结果。
type RequestBodyRead struct {
	StartedAt    time.Time
	FinishedAt   time.Time
	WireBytes    int64
	DecodedBytes int
	State        string
}

// WithRequestBodyRead 让入口日志读取处理器写入的正文读取信息。
func WithRequestBodyRead(ctx context.Context) (context.Context, *RequestBodyRead) {
	observation := &RequestBodyRead{}
	return context.WithValue(ctx, requestBodyReadKey{}, observation), observation
}

func ReadRequestBodyWithPrealloc(req *http.Request) ([]byte, error) {
	if req == nil || req.Body == nil {
		return nil, nil
	}

	capHint := requestBodyReadInitCap
	if req.ContentLength > 0 {
		switch {
		case req.ContentLength < int64(requestBodyReadInitCap):
			capHint = requestBodyReadInitCap
		case req.ContentLength > int64(requestBodyReadMaxInitCap):
			capHint = requestBodyReadMaxInitCap
		default:
			capHint = int(req.ContentLength)
		}
	}

	buf := bytes.NewBuffer(make([]byte, 0, capHint))
	observation, _ := req.Context().Value(requestBodyReadKey{}).(*RequestBodyRead)
	if observation != nil {
		observation.StartedAt = time.Now()
	}
	wireBytes, err := io.Copy(buf, req.Body)
	if observation != nil {
		observation.FinishedAt = time.Now()
		observation.WireBytes = wireBytes
		observation.State = "read_error"
	}
	if err != nil {
		return nil, err
	}
	decoded := buf.Bytes()
	enc := strings.ToLower(strings.TrimSpace(req.Header.Get("Content-Encoding")))
	if enc != "" && enc != "identity" {
		decoded, err = decompressRequestBody(enc, decoded)
		if err != nil {
			if observation != nil {
				observation.State = "decode_error"
			}
			return nil, fmt.Errorf("decode Content-Encoding %q: %w", enc, err)
		}
		req.Header.Del("Content-Encoding")
		req.Header.Del("Content-Length")
		req.ContentLength = int64(len(decoded))
	}
	if observation != nil {
		observation.State = "complete"
		observation.DecodedBytes = len(decoded)
	}
	return decoded, nil
}

func decompressRequestBody(encoding string, raw []byte) ([]byte, error) {
	switch encoding {
	case "zstd":
		dec, err := zstd.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer dec.Close()
		return io.ReadAll(io.LimitReader(dec, maxDecompressedBodySize))
	case "gzip", "x-gzip":
		gr, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = gr.Close()
		}()
		return io.ReadAll(io.LimitReader(gr, maxDecompressedBodySize))
	case "deflate":
		zr, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		defer func() {
			_ = zr.Close()
		}()
		return io.ReadAll(io.LimitReader(zr, maxDecompressedBodySize))
	default:
		return nil, errors.New("unsupported Content-Encoding")
	}
}

// BindJSONStrict 绑定单个 JSON 对象，拒绝未知字段，并按 binding 标签校验结构体。
// credentials、extra 等 map 字段的内容由调用方校验。
func BindJSONStrict(c *gin.Context, target any) error {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("request must contain one JSON object")
	}
	return binding.Validator.ValidateStruct(target)
}
