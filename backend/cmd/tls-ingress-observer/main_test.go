package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/infra/httpclient/tlsfingerprint"
)

func TestValidateObserverEndpoint(t *testing.T) {
	t.Parallel()
	endpoint, err := validateObserverEndpoint("http://127.0.0.1:8080" + observerEndpointPath)
	require.NoError(t, err)
	require.Equal(t, observerEndpointPath, endpoint.Path)
	_, err = validateObserverEndpoint("http://example.com" + observerEndpointPath)
	require.ErrorContains(t, err, "loopback")
	_, err = validateObserverEndpoint("http://127.0.0.1:8080/other")
	require.ErrorContains(t, err, "exact path")
}

func TestObservationPublisherPostsBearerAuthenticatedSafePayload(t *testing.T) {
	t.Parallel()
	token := strings.Repeat("t", 32)
	received := make(chan tlsfingerprint.AuthenticatedIngressObservation, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		require.Equal(t, observerEndpointPath, request.URL.Path)
		require.Equal(t, "Bearer "+token, request.Header.Get("Authorization"))
		var observation tlsfingerprint.AuthenticatedIngressObservation
		require.NoError(t, json.NewDecoder(request.Body).Decode(&observation))
		received <- observation
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	endpoint, err := validateObserverEndpoint(server.URL + observerEndpointPath)
	require.NoError(t, err)
	publisher := newObservationPublisher(endpoint, token, time.Second)
	want := &tlsfingerprint.AuthenticatedIngressObservation{
		SchemaVersion: 1,
		FlowID:        strings.Repeat("a", 64),
		ObservedAt:    time.Unix(1, 0).UTC(),
		ServerName:    "api.scienceedu.me",
		ClientHello:   &tlsfingerprint.CapturedClientHello{JA3Hash: "candidate"},
	}
	require.NoError(t, publisher.publish(context.Background(), want))
	require.Equal(t, *want, <-received)
}
