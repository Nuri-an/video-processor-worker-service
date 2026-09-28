package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rabbitmq/amqp091-go"
)

func TestEnvOr(t *testing.T) {
	result := envOr("NON_EXISTENT_VAR_XYZ", "default")
	if result != "default" {
		t.Errorf("envOr() = %q, want %q", result, "default")
	}

	t.Setenv("TEST_VAR_XYZ", "value")
	result = envOr("TEST_VAR_XYZ", "default")
	if result != "value" {
		t.Errorf("envOr() = %q, want %q", result, "value")
	}
}

func TestNotificationRecipient(t *testing.T) {
	tests := []struct {
		job      VideoJob
		expected string
	}{
		{VideoJob{User: "user1", Email: "user@email.com"}, "user@email.com"},
		{VideoJob{User: "user@email.com", Email: ""}, "user@email.com"},
		{VideoJob{User: "user1", Email: ""}, ""},
	}

	for _, tt := range tests {
		result := notificationRecipient(tt.job)
		if result != tt.expected {
			t.Errorf("notificationRecipient() = %q, want %q", result, tt.expected)
		}
	}
}

func TestRetryAttempt(t *testing.T) {
	tests := []struct {
		headers  map[string]interface{}
		expected int
	}{
		{map[string]interface{}{}, 0},
		{map[string]interface{}{"x-retry-attempt": int(1)}, 1},
		{map[string]interface{}{"x-retry-attempt": int32(2)}, 2},
		{map[string]interface{}{"x-retry-attempt": int64(3)}, 3},
		{map[string]interface{}{"x-retry-attempt": "invalid"}, 0},
	}

	for _, tt := range tests {
		msg := mockDelivery(tt.headers)
		result := retryAttempt(msg)
		if result != tt.expected {
			t.Errorf("retryAttempt() = %d, want %d", result, tt.expected)
		}
	}
}

func TestCreateZip(t *testing.T) {
	tmpDir := t.TempDir()

	file1 := filepath.Join(tmpDir, "frame_0001.png")
	file2 := filepath.Join(tmpDir, "frame_0002.png")
	os.WriteFile(file1, []byte("fake png 1"), 0644)
	os.WriteFile(file2, []byte("fake png 2"), 0644)

	zipPath := filepath.Join(tmpDir, "output.zip")
	err := createZip([]string{file1, file2}, zipPath)
	if err != nil {
		t.Fatalf("createZip() error = %v", err)
	}

	if _, err := os.Stat(zipPath); err != nil {
		t.Errorf("zip file not created: %v", err)
	}
}

func TestWorkerConcurrency(t *testing.T) {
	result := workerConcurrency()
	if result < 1 {
		t.Errorf("workerConcurrency() = %d, want >= 1", result)
	}
}

func mockDelivery(headers map[string]interface{}) amqp091.Delivery {
	amqpTable := make(amqp091.Table)
	for k, v := range headers {
		amqpTable[k] = v
	}
	return amqp091.Delivery{Headers: amqpTable}
}