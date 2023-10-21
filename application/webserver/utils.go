package gatesentryWebserver

import (
	"encoding/json"
	"errors"
	"net/http"
)

func ParseJSONRequest(r *http.Request, v interface{}) error {
	defer r.Body.Close() // Ensure the body is closed after reading
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Optionally disallow unknown fields for stricter parsing
	if err := decoder.Decode(v); err != nil {
		var syntaxErr *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError

		switch {
		case errors.As(err, &syntaxErr):
			return errors.New("malformed JSON at " + string(rune(syntaxErr.Offset)))
		case errors.As(err, &unmarshalTypeError):
			return errors.New("JSON value of type " + unmarshalTypeError.Value + " cannot be converted to Go value type " + unmarshalTypeError.Type.String())
		case errors.As(err, &invalidUnmarshalError):
			return errors.New("cannot unmarshal into unaddressable value")
		default:
			return err
		}
	}

	// Check for remaining data in the body
	if decoder.More() {
		return errors.New("request body contains unexpected extra data")
	}

	return nil
}

func SendError(w http.ResponseWriter, err error, statusCode int) {
	w.WriteHeader(statusCode)
	errorResponse := ErrorResponse{
		StatusCode:   statusCode,
		ErrorMessage: err.Error(),
	}
	json.NewEncoder(w).Encode(errorResponse)
}

func SendJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
