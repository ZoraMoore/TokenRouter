package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const revivetokenTestAccountID = "a123456789abcdef0123456789abcdef"

func TestReviveTokenFindsUniqueExactEmailAcrossPages(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret" || r.URL.Query().Get("email") != "user+2@example.org" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("page") {
		case "1":
			_, _ = io.WriteString(w, `{"data":{"total":101,"items":[{"account_id":"`+revivetokenTestAccountID+`","email":"USER+2@EXAMPLE.ORG"}]}}`)
		case "2":
			_, _ = io.WriteString(w, `{"total":101,"items":[{"account_id":"`+revivetokenTestAccountID+`","email":"user+2@example.org"},{"account_id":"b123456789abcdef0123456789abcdef","email":"anotheruser+2@example.org"}]}`)
		default:
			http.Error(w, "bad page", http.StatusBadRequest)
		}
	}))
	defer srv.Close()
	client := &ReviveClient{baseURL: srv.URL, apiKey: "test-secret", http: srv.Client()}
	id, err := client.FindAccount(ctx, "user+2@example.org")
	require.NoError(t, err)
	require.Equal(t, revivetokenTestAccountID, id)
}

func TestReviveTokenMutationRequires202AndDoesNotReturnRemoteBody(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil || len(body) != 0 || r.Header.Get("Idempotency-Key") != "fixture-delete" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/pool-account") && r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"operation_id":"d123456789abcdef0123456789abcdef"}`)
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"message":"database-private-value"}`)
	}))
	defer srv.Close()
	client := &ReviveClient{baseURL: srv.URL, apiKey: "test-secret", http: srv.Client()}
	id, err := client.DeletePoolAccount(ctx, revivetokenTestAccountID, "fixture-delete")
	require.NoError(t, err)
	require.Equal(t, "d123456789abcdef0123456789abcdef", id)
	_, err = client.SyncAccount(ctx, revivetokenTestAccountID, "fixture-delete")
	require.ErrorContains(t, err, "403")
	require.NotContains(t, err.Error(), "database-private-value")
}
