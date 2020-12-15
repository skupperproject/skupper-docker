package client

import (
	"os"
	"testing"

	"gotest.tools/assert"
)

func TestNewClient(t *testing.T) {
	testCases := []struct {
		doc             string
		expectedError   string
		expectedVersion string
	}{
		{
			expectedError: "",
			doc:           "test one",
		},
	}

	for _, c := range testCases {
		_, err := NewClient()
		assert.Check(t, err, c.doc)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
