package dynamodb

import (
	"context"
	"errors"
	"testing"
)

func TestNewClient(t *testing.T) {
	_, err := NewClient(context.Background(), "profile", "region", "tableName", "keyARN")
	if err != nil {
		if err.Error() != "failed to get shared config profile, profile" {
			t.Error(err)
		}
	} else {
		t.Error(errors.New("expected an error getting a shared config profile named profile"))
	}
}
