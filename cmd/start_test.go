package cmd

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRunStart_Success(t *testing.T) {
	mockClient := new(MockClient)
	mockClient.On("Start", mock.Anything).Return(nil)

	var buf bytes.Buffer
	err := runStart(context.Background(), mockClient, &buf)

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "✓ Service started successfully")
	mockClient.AssertExpectations(t)
}

func TestRunStart_AlreadyRunning(t *testing.T) {
	mockClient := new(MockClient)
	mockClient.On("Start", mock.Anything).Return(errors.New("service is already running"))

	var buf bytes.Buffer
	err := runStart(context.Background(), mockClient, &buf)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
	mockClient.AssertExpectations(t)
}
