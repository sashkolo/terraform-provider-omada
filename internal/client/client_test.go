package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_Success(t *testing.T) {
	var receivedGrant, receivedClientId, receivedAuth string
	accessToken := "mock-access-token"
	controllerId := "mock-controller-id"
	clientId := "mock-client-id"
	clientSecret := "mock-client-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/probe" {
			receivedAuth = r.Header.Get("Authorization")
			return
		}
		receivedGrant = r.URL.Query().Get("grant_type")

		var body struct {
			ClientId string `json:"client_id"`
		}

		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedClientId = body.ClientId

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errorCode": 0,
			"result":    map[string]any{"accessToken": accessToken},
		})
	}))

	// Close the server after the test runs
	t.Cleanup(server.Close)

	meta, err := New(context.Background(), Config{
		Host:         server.URL,
		ControllerID: controllerId,
		ClientID:     clientId,
		ClientSecret: clientSecret,
	})

	if err != nil {
		t.Fatal(err)
	}

	if meta.OmadacId != controllerId {
		t.Errorf("OmadaID value: %q", meta.OmadacId)
	}

	if receivedGrant != "client_credentials" {
		t.Errorf("Grant type: %q", receivedGrant)
	}

	if receivedClientId != clientId {
		t.Errorf("Client ID: %q", receivedClientId)
	}

	// The token is added per request by the client's transport.
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/probe", nil)
	resp, err := meta.Client.GetConfig().HTTPClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if receivedAuth != fmt.Sprintf("AccessToken=%s", accessToken) {
		t.Errorf("Received Token: %q", receivedAuth)
	}
}

func TestNew_AuthFailure(t *testing.T) {
	controllerId := "mock-controller-id"
	clientId := "mock-client-id"
	clientSecret := "mock-client-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errorCode": -1001,
			"msg":       "Could not get access token",
		})
	}))

	// Close the server after the test runs
	t.Cleanup(server.Close)

	_, err := New(context.Background(), Config{
		Host:         server.URL,
		ControllerID: controllerId,
		ClientID:     clientId,
		ClientSecret: clientSecret,
	})

	if err == nil {
		t.Fatal("No error returned")
	}

	// The controller's own reason is surfaced, not only the HTTP status.
	if err.Error() != "the controller refused the access token request, error code -1001: Could not get access token" {
		t.Errorf("Received Error: %s", err.Error())
	}
}

func TestNew_NoResult(t *testing.T) {
	controllerId := "mock-controller-id"
	clientId := "mock-client-id"
	clientSecret := "mock-client-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errorCode": 0,
			"msg":       "No result",
		})
	}))

	// Close the server after the test runs
	t.Cleanup(server.Close)

	_, err := New(context.Background(), Config{
		Host:         server.URL,
		ControllerID: controllerId,
		ClientID:     clientId,
		ClientSecret: clientSecret,
	})

	if err == nil {
		t.Fatal("No error returned")
	}

	if err.Error() != "token response missing result" {
		t.Errorf("Received Error: %s", err.Error())
	}
}

func TestNew_NoAccessToken(t *testing.T) {
	controllerId := "mock-controller-id"
	clientId := "mock-client-id"
	clientSecret := "mock-client-secret"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errorCode": 0,
			"result":    map[string]any{},
		})
	}))

	// Close the server after the test runs
	t.Cleanup(server.Close)

	_, err := New(context.Background(), Config{
		Host:         server.URL,
		ControllerID: controllerId,
		ClientID:     clientId,
		ClientSecret: clientSecret,
	})

	if err == nil {
		t.Fatal("No error returned")
	}

	if err.Error() != "token response missing access token" {
		t.Errorf("Received Error: %s", err.Error())
	}
}
