package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
)

const (
	observerEndpointPath = "/internal/tls-fingerprint/authenticated-ingress/observations"
	defaultEndpoint      = "http://127.0.0.1:8080" + observerEndpointPath
	defaultServerName    = "api.scienceedu.me"
	defaultFlowTTL       = 300 * time.Second
	defaultMaxFlows      = 4096
	defaultMaxFlowBytes  = 256 << 10
	defaultMaxTotalBytes = 32 << 20
	defaultQueueSize     = 256
	defaultWorkers       = 1
	defaultHTTPTimeout   = 3 * time.Second
	defaultSocketBuffer  = 4 << 20
	packetBufferBytes    = 65536
)

type observerConfig struct {
	endpoint          *url.URL
	token             string
	interfaceName     string
	serverName        string
	flowTTL           time.Duration
	maxFlows          int
	maxFlowBytes      int
	maxTotalBytes     int
	queueSize         int
	publisherWorkers  int
	httpTimeout       time.Duration
	socketBufferBytes int
}

type observationPublisher struct {
	endpoint *url.URL
	token    string
	client   *http.Client
}

func main() {
	logger := log.New(os.Stderr, "tls-ingress-observer: ", log.LstdFlags|log.LUTC)
	config, err := loadObserverConfig()
	if err != nil {
		logger.Fatalf("configuration error: %v", err)
	}
	assembler, err := newFlowAssembler(assemblerLimits{
		ttl:              config.flowTTL,
		maxFlows:         config.maxFlows,
		maxFlowBytes:     config.maxFlowBytes,
		maxBufferedBytes: config.maxTotalBytes,
		serverName:       config.serverName,
		observerToken:    config.token,
	})
	if err != nil {
		logger.Fatalf("assembler configuration error: %v", err)
	}
	capture, resolvedInterface, err := openPacketCapture(config.interfaceName, config.socketBufferBytes)
	if err != nil {
		logger.Fatalf("capture initialization failed: %v", err)
	}
	defer func() { _ = capture.Close() }()
	if err := dropObserverPrivileges(); err != nil {
		logger.Fatalf("privilege drop failed: %v", err)
	}
	if actual := capture.ReceiveBufferBytes(); actual < config.socketBufferBytes {
		logger.Printf("warning: packet receive buffer clamped requested=%d actual=%d", config.socketBufferBytes, actual)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = capture.Close()
	}()

	queue := make(chan tlsfingerprint.AuthenticatedIngressObservation, config.queueSize)
	publisher := newObservationPublisher(config.endpoint, config.token, config.httpTimeout)
	for range config.publisherWorkers {
		go publisher.run(ctx, queue, logger)
	}
	logger.Printf("started interface=%s server_name=%s destination_port=443 uid=%d", resolvedInterface, config.serverName, os.Geteuid())

	packetBuffer := make([]byte, packetBufferBytes)
	var lastQueueFullLog time.Time
	queueDrops := 0
	lastPacketStats := time.Now()
	for {
		count, readErr := capture.Read(ctx, packetBuffer)
		if readErr != nil {
			if ctx.Err() != nil || errors.Is(readErr, syscall.EBADF) {
				logger.Print("stopped")
				return
			}
			logger.Fatalf("packet capture failed: %v", readErr)
		}
		segment, ok := decodeEthernetTCPPacket(packetBuffer[:count], 443)
		if time.Since(lastPacketStats) >= time.Minute {
			packets, drops, statsErr := capture.Statistics()
			if statsErr != nil {
				logger.Printf("packet statistics unavailable: %v", statsErr)
			} else if drops > 0 {
				logger.Printf("packet capture drops detected packets=%d drops=%d", packets, drops)
			}
			lastPacketStats = time.Now()
		}
		if !ok {
			continue
		}
		observation := assembler.add(segment, time.Now())
		if observation == nil {
			continue
		}
		select {
		case queue <- *observation:
		default:
			// 队列饱和时丢弃观测而不是阻塞 Nginx 或扩张内存。
			queueDrops++
			now := time.Now()
			if lastQueueFullLog.IsZero() || now.Sub(lastQueueFullLog) >= 30*time.Second {
				logger.Printf("publisher queue full; observations dropped=%d", queueDrops)
				lastQueueFullLog = now
				queueDrops = 0
			}
		}
	}
}

func loadObserverConfig() (observerConfig, error) {
	endpoint, err := validateObserverEndpoint(environmentValue("AUTHENTICATED_INGRESS_OBSERVER_ENDPOINT", defaultEndpoint))
	if err != nil {
		return observerConfig{}, err
	}
	token := strings.TrimSpace(os.Getenv("AUTHENTICATED_INGRESS_OBSERVER_TOKEN"))
	if len(token) < 32 {
		return observerConfig{}, errors.New("AUTHENTICATED_INGRESS_OBSERVER_TOKEN must contain at least 32 bytes")
	}
	serverName := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(environmentValue("AUTHENTICATED_INGRESS_OBSERVER_SERVER_NAME", defaultServerName)), "."))
	if !validObserverServerName(serverName) {
		return observerConfig{}, errors.New("AUTHENTICATED_INGRESS_OBSERVER_SERVER_NAME must be a DNS hostname")
	}
	flowTTLSeconds, err := integerEnvironment("AUTHENTICATED_INGRESS_OBSERVER_FLOW_TTL_SECONDS", int(defaultFlowTTL.Seconds()), 10, 600)
	if err != nil {
		return observerConfig{}, err
	}
	maxFlows, err := integerEnvironment("AUTHENTICATED_INGRESS_OBSERVER_MAX_FLOWS", defaultMaxFlows, 64, 65536)
	if err != nil {
		return observerConfig{}, err
	}
	maxFlowBytes, err := integerEnvironment("AUTHENTICATED_INGRESS_OBSERVER_MAX_FLOW_BYTES", defaultMaxFlowBytes, 4096, 512<<10)
	if err != nil {
		return observerConfig{}, err
	}
	maxTotalBytes, err := integerEnvironment("AUTHENTICATED_INGRESS_OBSERVER_MAX_TOTAL_BYTES", defaultMaxTotalBytes, maxFlowBytes, 512<<20)
	if err != nil {
		return observerConfig{}, err
	}
	queueSize, err := integerEnvironment("AUTHENTICATED_INGRESS_OBSERVER_QUEUE_SIZE", defaultQueueSize, 1, 8192)
	if err != nil {
		return observerConfig{}, err
	}
	workers, err := integerEnvironment("AUTHENTICATED_INGRESS_OBSERVER_PUBLISHER_WORKERS", defaultWorkers, 1, 16)
	if err != nil {
		return observerConfig{}, err
	}
	httpTimeoutMillis, err := integerEnvironment("AUTHENTICATED_INGRESS_OBSERVER_HTTP_TIMEOUT_MILLISECONDS", int(defaultHTTPTimeout.Milliseconds()), 100, 30000)
	if err != nil {
		return observerConfig{}, err
	}
	socketBufferBytes, err := integerEnvironment("AUTHENTICATED_INGRESS_OBSERVER_SOCKET_BUFFER_BYTES", defaultSocketBuffer, 64<<10, 64<<20)
	if err != nil {
		return observerConfig{}, err
	}
	return observerConfig{
		endpoint:          endpoint,
		token:             token,
		interfaceName:     strings.TrimSpace(os.Getenv("AUTHENTICATED_INGRESS_OBSERVER_INTERFACE")),
		serverName:        serverName,
		flowTTL:           time.Duration(flowTTLSeconds) * time.Second,
		maxFlows:          maxFlows,
		maxFlowBytes:      maxFlowBytes,
		maxTotalBytes:     maxTotalBytes,
		queueSize:         queueSize,
		publisherWorkers:  workers,
		httpTimeout:       time.Duration(httpTimeoutMillis) * time.Millisecond,
		socketBufferBytes: socketBufferBytes,
	}, nil
}

func validateObserverEndpoint(raw string) (*url.URL, error) {
	endpoint, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || endpoint == nil || endpoint.Hostname() == "" {
		return nil, errors.New("AUTHENTICATED_INGRESS_OBSERVER_ENDPOINT must be an absolute HTTP(S) URL")
	}
	if endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" || endpoint.RawPath != "" || endpoint.Path != observerEndpointPath {
		return nil, fmt.Errorf("observer endpoint must use the exact path %s without credentials, query, or fragment", observerEndpointPath)
	}
	switch strings.ToLower(endpoint.Scheme) {
	case "https":
	case "http":
	default:
		return nil, errors.New("observer endpoint scheme must be http or https")
	}
	address := net.ParseIP(endpoint.Hostname())
	if address == nil && strings.EqualFold(endpoint.Hostname(), "localhost") {
		address = net.IPv4(127, 0, 0, 1)
	}
	if address == nil || !address.IsLoopback() {
		return nil, errors.New("observer endpoint must use a loopback host")
	}
	return endpoint, nil
}

func validObserverServerName(value string) bool {
	if value == "" || len(value) > 253 {
		return false
	}
	for label := range strings.SplitSeq(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func environmentValue(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func integerEnvironment(name string, fallback, minimum, maximum int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be an integer between %d and %d", name, minimum, maximum)
	}
	return value, nil
}

func newObservationPublisher(endpoint *url.URL, token string, timeout time.Duration) *observationPublisher {
	return &observationPublisher{
		endpoint: endpoint,
		token:    token,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy:                 nil,
				DisableCompression:    true,
				MaxIdleConns:          4,
				MaxIdleConnsPerHost:   4,
				IdleConnTimeout:       30 * time.Second,
				ResponseHeaderTimeout: timeout,
			},
		},
	}
}

func (p *observationPublisher) run(ctx context.Context, queue <-chan tlsfingerprint.AuthenticatedIngressObservation, logger *log.Logger) {
	var lastErrorLog time.Time
	suppressedErrors := 0
	for {
		select {
		case <-ctx.Done():
			return
		case observation := <-queue:
			if err := p.publish(ctx, &observation); err != nil {
				// 错误只包含端点状态，不记录 flow ID、ClientHello 或 token。
				now := time.Now()
				if lastErrorLog.IsZero() || now.Sub(lastErrorLog) >= 30*time.Second {
					logger.Printf("observation publish failed: %v (suppressed=%d)", err, suppressedErrors)
					lastErrorLog = now
					suppressedErrors = 0
				} else {
					suppressedErrors++
				}
			}
		}
	}
}

func (p *observationPublisher) publish(ctx context.Context, observation *tlsfingerprint.AuthenticatedIngressObservation) error {
	raw, err := json.Marshal(observation)
	if err != nil {
		return errors.New("encode observation")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint.String(), bytes.NewReader(raw))
	if err != nil {
		return errors.New("create observation request")
	}
	request.Header.Set("Authorization", "Bearer "+p.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "tokenrouter-tls-ingress-observer/1")
	response, err := p.client.Do(request)
	if err != nil {
		return errors.New("internal endpoint unavailable")
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("internal endpoint returned HTTP %d", response.StatusCode)
	}
	return nil
}
