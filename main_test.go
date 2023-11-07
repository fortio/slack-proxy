package main

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestGetSlackTokens(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected []string
	}{
		{
			name:     "Multiple tokens",
			envValue: "token1,token2,token3",
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
			os.Setenv("SLACK_TOKENS", tt.envValue)

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
		envValue  string
		expected  int
		expectErr error
	}{
		{
			name:      "Valid pod name",
			envValue:  "pod-3",
			expected:  3,
			expectErr: nil,
		},
		{
			name:      "Invalid pod name",
			envValue:  "pod",
			expected:  0,
			expectErr: fmt.Errorf("invalid pod name %s. Expected <name>-<index>", "pod"),
		},
		{
			name:      "Invalid pod index",
			envValue:  "pod-abcde",
			expected:  0,
			expectErr: fmt.Errorf("invalid pod name format. Expected <name>-<index>, got %s", "pod-abcde"),
		},
		{
			name:      "No pod name",
			envValue:  "",
			expected:  0,
			expectErr: errors.New("HOSTNAME environment variable not set"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the environment variable for the test
			os.Setenv("HOSTNAME", tt.envValue)

			// Call the function
			index, err := podIndex()

			// Check if an error was expected
			if tt.expectErr != nil && err.Error() != tt.expectErr.Error() {
				t.Errorf("Expected error %v, but got %v", tt.expectErr, err)
			}

			// Check if the result matches the expected output
			if index != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, index)
			}

			// Clean up the environment variable
			os.Unsetenv("HOSTNAME")
		})
	}
}
