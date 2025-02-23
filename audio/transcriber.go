package audio

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"lazywhisper/config"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
)

type Transcriber struct {
	apiKey     string
	appDataDir string
}

type TranscriptionResponse struct {
	Text string `json:"text"`
}

func NewTranscriber(apiKey string) *Transcriber {
	// Get app data directory
	appDataDir, err := config.GetAppDataDir()
	if err != nil {
		log.Fatal(err)
	}

	return &Transcriber{
		apiKey:     apiKey,
		appDataDir: appDataDir,
	}
}

func (t *Transcriber) Transcribe(audioFile string) (string, error) {
	file, err := os.Open(audioFile)
	if err != nil {
		return "", fmt.Errorf("failed to open audio file: %w", err)
	}
	defer file.Close()

	// Create a buffer to store the multipart form data
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add the file field
	part, err := writer.CreateFormFile("file", filepath.Base(audioFile))
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("failed to copy file data: %w", err)
	}

	// Add the model field
	if err := writer.WriteField("model", "whisper-1"); err != nil {
		return "", fmt.Errorf("failed to write model field: %w", err)
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("failed to close writer: %w", err)
	}

	// Create the request
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/audio/transcriptions", &buf)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the response
	var result TranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Save transcription to file
	timestamp := filepath.Base(audioFile[:len(audioFile)-4]) // Remove .wav extension
	transcriptionFile := filepath.Join(t.appDataDir, config.TranscriptionsDir, timestamp+".txt")
	if err := os.WriteFile(transcriptionFile, []byte(result.Text), 0644); err != nil {
		return "", fmt.Errorf("failed to save transcription: %w", err)
	}

	return result.Text, nil
} 