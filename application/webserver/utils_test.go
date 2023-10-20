package gatesentryWebserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type DummyData struct {
	Field string `json:"field"`
}

func TestParseJSONRequest(t *testing.T) {
	req := httptest.NewRequest("POST", "/", bytes.NewBuffer([]byte(`{"field": "value"}`)))
	var data DummyData

	if err := ParseJSONRequest(req, &data); err != nil {
		t.Fatal(err)
	}

	if data.Field != "value" {
		t.Fatalf("Expected value, got %s", data.Field)
	}
}

func TestSendError(t *testing.T) {
	recorder := httptest.NewRecorder()
	err := errors.New("some error")

	SendError(recorder, err, http.StatusBadRequest)

	result := recorder.Result()
	defer result.Body.Close()

	if result.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d", result.StatusCode)
	}
}

func TestSendJSON(t *testing.T) {
	recorder := httptest.NewRecorder()
	data := DummyData{Field: "value"}

	SendJSON(recorder, data)

	result := recorder.Result()
	defer result.Body.Close()

	var respData DummyData
	json.NewDecoder(result.Body).Decode(&respData)

	if respData.Field != "value" {
		t.Fatalf("Expected value, got %s", respData.Field)
	}
}
