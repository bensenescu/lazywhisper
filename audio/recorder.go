package audio

import (
	"fmt"
	"lazywhisper/config"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Recorder struct {
	cmd         *exec.Cmd
	outputFile  string
	isRecording bool
	appDataDir  string
	timer       *time.Timer
}

func NewRecorder() *Recorder {
	// Get app data directory
	appDataDir, err := config.GetAppDataDir()
	if err != nil {
		log.Fatal(err)
	}

	return &Recorder{
		isRecording: false,
		appDataDir:  appDataDir,
	}
}

func (r *Recorder) StartRecording() error {
	if r.isRecording {
		return fmt.Errorf("recording is already in progress")
	}

	// Generate output filename with timestamp
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	r.outputFile = filepath.Join(r.appDataDir, config.RecordingsDir, fmt.Sprintf("%s.wav", timestamp))

	// Start ffmpeg process with stderr piped to null to avoid noise
	r.cmd = exec.Command("ffmpeg",
		"-f", "avfoundation",
		"-i", ":1",
		"-y", // Overwrite output file if it exists
		r.outputFile,
	)
	r.cmd.Stderr = nil

	// Start the recording process
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start recording: %w", err)
	}

	// Start 20-minute timer
	r.timer = time.NewTimer(20 * time.Minute)
	go func() {
		<-r.timer.C
		if r.isRecording {
			_ = r.StopRecording() // Ignore error since this is a background operation
		}
	}()

	r.isRecording = true
	return nil
}

func (r *Recorder) StopRecording() error {
	if !r.isRecording {
		return fmt.Errorf("no recording in progress")
	}

	// Stop and clean up timer if it exists
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}

	// Try to gracefully stop the current recording process
	if r.cmd != nil && r.cmd.Process != nil {
		// Send SIGINT to ffmpeg for graceful shutdown
		if err := r.cmd.Process.Signal(os.Interrupt); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to interrupt ffmpeg process: %v\n", err)
			// If interrupting fails, try to kill the process
			_ = r.cmd.Process.Kill()
		}

		// Wait for the process to finish with a timeout
		done := make(chan error, 1)
		go func() {
			done <- r.cmd.Wait()
		}()

		// Wait for process to exit with a 2-second timeout
		select {
		case <-done:
			// Process exited normally
		case <-time.After(2 * time.Second):
			// Process didn't exit in time, force kill it
			_ = r.cmd.Process.Kill()
		}
	}

	// Find and kill any other ffmpeg processes recording to .open_whisper
	r.killOrphanedFFmpegProcesses()

	// Wait for the file to exist (up to 2 seconds)
	for i := 0; i < 20; i++ {
		if _, err := os.Stat(r.outputFile); err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	r.isRecording = false
	r.cmd = nil
	return nil
}

// Cleanup finds and kills any orphaned ffmpeg processes
func Cleanup() {
	// Create a temporary recorder to access the cleanup method
	r := &Recorder{}
	r.killOrphanedFFmpegProcesses()
}

// killOrphanedFFmpegProcesses finds and kills any ffmpeg processes recording to .open_whisper
func (r *Recorder) killOrphanedFFmpegProcesses() {
	// Find all ffmpeg processes
	cmd := exec.Command("ps", "-eo", "pid,command")
	output, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to list processes: %v\n", err)
		return
	}

	// Parse the output to find ffmpeg processes recording to .open_whisper
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "ffmpeg") && strings.Contains(line, ".open_whisper") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}

			// Extract PID
			pid := fields[0]
			if pid == fmt.Sprintf("%d", os.Getpid()) {
				// Skip our own process
				continue
			}

			// Try to kill the process
			killCmd := exec.Command("kill", "-INT", pid)
			if err := killCmd.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to interrupt ffmpeg process %s: %v\n", pid, err)
				// If interrupting fails, try to force kill
				forceKillCmd := exec.Command("kill", "-9", pid)
				_ = forceKillCmd.Run()
			}
		}
	}
}

func (r *Recorder) GetOutputFile() string {
	return r.outputFile
}

func (r *Recorder) IsRecording() bool {
	return r.isRecording
} 