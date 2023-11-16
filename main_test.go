package main

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"fortio.org/log"
)

func TestLoggerNotWorking(t *testing.T) {
	req := SlackPostMessageRequest{}

	err := validate(req)

	log.S(log.Error, "Testing logging", log.Any("err", err))
	log.S(log.Error, "Testing logging", log.Any("err", err.Error()))
}

func TestGetSlackTokens(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected []string
	}{
		{
			name:     "Multiple tokens",
			envValue: "token1,token2, token3",
			expected: []string{"token1", "token2", "token3"},
		},
		{
			name:     "Single token",
			envValue: "token1",
			expected: []string{"token1"},
		},
		{
			name:     "No tokens",
			envValue: "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the environment variable for the test
			t.Setenv("SLACK_TOKENS", tt.envValue)

			// Call the function
			tokens := getSlackTokens()

			// Check if the result matches the expected output
			if !reflect.DeepEqual(tokens, tt.expected) {
				t.Errorf("Expected %v, got %v", tt.expected, tokens)
			}

			// Clean up the environment variable
			os.Unsetenv("SLACK_TOKENS")
		})
	}
}

func TestPodIndex(t *testing.T) {
	tests := []struct {
		name      string
		podName   string
		expected  int
		expectErr error
	}{
		{
			name:      "Valid pod name",
			podName:   "pod-3",
			expected:  3,
			expectErr: nil,
		},
		{
			name:      "Invalid pod name, no index",
			podName:   "pod",
			expected:  0,
			expectErr: fmt.Errorf("invalid pod name %s. Expected <name>-<index>", "pod"),
		},
		{
			name:      "Invalid pod name, dash at the end",
			podName:   "pod-",
			expected:  0,
			expectErr: fmt.Errorf("invalid pod name %s. Expected <name>-<index>", "pod-"),
		},
		{
			name:      "Invalid pod index",
			podName:   "pod-abcde",
			expected:  0,
			expectErr: fmt.Errorf("invalid pod name format. Expected <name>-<index>, got %s", "pod-abcde"),
		},
		{
			name:      "No pod name",
			podName:   "",
			expected:  0,
			expectErr: fmt.Errorf("invalid pod name %s. Expected <name>-<index>", ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Call the function
			index, err := podIndex(tt.podName)

			// Check if an error was expected
			if tt.expectErr != nil && err.Error() != tt.expectErr.Error() {
				t.Errorf("Expected error %v, but got %v", tt.expectErr, err)
			}

			// Check if the result matches the expected output
			if index != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, index)
			}
		})
	}
}
