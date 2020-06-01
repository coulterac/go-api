package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestSendError(t *testing.T) {
	type testCase struct {
		name       string
		err        string
		statusCode int
		resp       apiError
	}

	cases := []testCase{
		testCase{
			name:       "internal server error",
			err:        "unit-test",
			statusCode: http.StatusInternalServerError,
			resp: apiError{
				Message: "unit-test",
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			sendError(rr, c.statusCode, c.err)

			var resp apiError
			err := json.NewDecoder(rr.Body).Decode(&resp)
			if err != nil {
				t.Error(err.Error())
			}

			if !reflect.DeepEqual(resp, c.resp) {
				t.Errorf("expected responses to match; got: %v, want: %v", resp, c.resp)
			}
			if rr.Code != c.statusCode {
				t.Errorf("expected status codes to match; got: %v, want %v", rr.Code, c.statusCode)
			}
		})
	}
}
