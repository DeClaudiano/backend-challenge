package httpadapter

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
)

type dependency struct{ err error }

func (d dependency) Ping(context.Context) error  { return d.err }
func (d dependency) Ready(context.Context) error { return d.err }

func TestLive(t *testing.T) {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/health/live", nil)
	live(recorder, req)
	if recorder.Code != 200 {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestReadinessChecksDependencies(t *testing.T) {
	tests := []struct {
		name            string
		poolErr, sqsErr error
		want            int
	}{
		{name: "ready", want: 200},
		{name: "postgres unavailable", poolErr: errors.New("down"), want: 503},
		{name: "sqs unavailable", sqsErr: errors.New("down"), want: 503},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/health/ready", nil)
			readiness(dependency{tc.poolErr}, dependency{tc.sqsErr})(recorder, req)
			if recorder.Code != tc.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.want)
			}
		})
	}
}
