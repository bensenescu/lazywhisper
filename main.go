package main

// A simple program demonstrating the text area component from the Bubbles
// component library.

import (
	"fmt"
	"lazywhisper/audio"
	"lazywhisper/config"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	gap = "\n\n"
	setupInstructions = `Setup Required:

OpenAI API Key Setup:
1. Get an API key from https://platform.openai.com/account/api-keys
2. Set it as an environment variable:
   export OPENAI_API_KEY='your-api-key'
   
FFmpeg Setup:
1. Install FFmpeg using your package manager:
   • macOS (using Homebrew):
     brew install ffmpeg
   • Ubuntu/Debian:
     sudo apt-get install ffmpeg
   • Windows:
     Install from https://ffmpeg.org/download.html

After setting up, restart the application.`

 smallMicrophone = `
     @@@@@@@
   @@@@@@@@@@@
   @@@@@@@@@@@
  @@@@@@@@@@@@@
  @@@@@@@@@@@@@
  @@@@@@@@@@@@@
  @@@@@@@@@@@@@
@@@@@@@@@@@@@@@@@@
@@@ @@@@@@@@@@ @@@
@@@ @@@@@@@@@@ @@@
 @@@@ @@@@@@ @@@@
  @@@@@@@@@@@@@@
    @@@@@@@@@
       @@                
       @@
    @@@@@@@@@


`
)

var (
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	paddedStyle = lipgloss.NewStyle().PaddingLeft(2)
	successStyleWithPadding = lipgloss.NewStyle().Foreground(lipgloss.Color("2")).PaddingLeft(2)
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	// Match help style to the default color scheme
	helpStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
		Light: "#B2B2B2",
		Dark:  "#4A4A4A",
	})
)

type recordingStartedMsg struct{}
type recordingStoppedMsg struct{ err error }
type transcriptionFinishedMsg struct {
	text string
	err  error
}

type copyToClipboardMsg struct{ err error }

type tickMsg struct{}

func checkDependencies() error {
	// Check OpenAI API key
	if os.Getenv("OPENAI_API_KEY") == "" {
		return fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}

	// Check FFmpeg availability
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg is not installed or not in PATH")
	}

	return nil
}

func main() {
	// Set up cleanup for when the program exits
	setupCleanup()

	// Check dependencies first
	if err := checkDependencies(); err != nil {
		fmt.Printf("\n%s\n\n", errorStyle.Render(fmt.Sprintf("Error: %v", err)))
		fmt.Println(setupInstructions)
		os.Exit(1)
	}

	// Get OpenAI API key from environment
	apiKey := os.Getenv("OPENAI_API_KEY")

	p := tea.NewProgram(
		initialModel(apiKey),
		tea.WithAltScreen(),       // Use alternate screen buffer
		tea.WithMouseCellMotion(), // Enable mouse support
	)

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}
	
	audio.Cleanup()
}

// setupCleanup registers signal handlers to ensure we clean up ffmpeg processes on exit
func setupCleanup() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		audio.Cleanup()
		os.Exit(0)
	}()
}

type (
	errMsg error
)

// keyMap defines a set of keybindings. To work for help it must satisfy
// key.Map. It could also very easily be a map[string]key.Binding.
type keyMap struct {
	Record        key.Binding
	StopRecording key.Binding
	CopyToClip    key.Binding
	ListTranscriptions key.Binding
	Help          key.Binding
	Back          key.Binding
	Up            key.Binding
	Down          key.Binding
	Quit          key.Binding
	Delete        key.Binding
	Confirm       key.Binding
}

var keys = keyMap{
	Record: key.NewBinding(
		key.WithKeys("r"),
		key.WithHelp("<r>", "Record"),
	),
	StopRecording: key.NewBinding(
		key.WithKeys(" ", "enter"),
		key.WithHelp("<space>", "Stop recording"),
	),
	CopyToClip: key.NewBinding(
		key.WithKeys("c", "y"),
		key.WithHelp("<c/y>", "Copy transcription"),
	),
	ListTranscriptions: key.NewBinding(
		key.WithKeys("l"),
		key.WithHelp("<l>", "List transcriptions"),
	),
	Help: key.NewBinding(
		key.WithKeys("?"),
		key.WithHelp("<?>", "Toggle help"),
	),
	Back: key.NewBinding(
		key.WithKeys("esc"),
		key.WithHelp("<esc>", "Back"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "k"),
		key.WithHelp("<↑/k>", "Move up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "j"),
		key.WithHelp("<↓/j>", "Move down"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("<q>", "Quit"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("<d>", "Delete"),
	),
	Confirm: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("<enter>", "Confirm"),
	),
}

type RecordingState int

const (
	Idle RecordingState = iota
	Recording
	Transcribing
	TranscriptionComplete
)

type model struct {
	viewport       viewport.Model
	help          help.Model
	recordingState RecordingState
	senderStyle   lipgloss.Style
	err           error
	recorder      *audio.Recorder
	transcriber   *audio.Transcriber
	transcription string
	showCopied    bool
	width         int
	height        int
	showingTranscriptions bool
	transcriptionFiles    []string
	selectedIndex        int
	selectedContent      string
	showingDeleteConfirmation bool
}

func loadTranscriptionContent(filename string) (string, error) {
	appDataDir, err := config.GetAppDataDir()
	if err != nil {
		return "", fmt.Errorf("failed to get app data directory: %w", err)
	}

	transcriptionPath := filepath.Join(appDataDir, config.TranscriptionsDir, filename)
	content, err := os.ReadFile(transcriptionPath)
	if err != nil {
		return "", fmt.Errorf("failed to read transcription: %w", err)
	}

	return string(content), nil
}

func loadTranscriptions() tea.Msg {
	appDataDir, err := config.GetAppDataDir()
	if err != nil {
		return errMsg(fmt.Errorf("failed to get app data directory: %w", err))
	}

	transcriptionsPath := filepath.Join(appDataDir, config.TranscriptionsDir)
	files, err := os.ReadDir(transcriptionsPath)
	if err != nil {
		return errMsg(fmt.Errorf("failed to read transcriptions directory: %w", err))
	}

	var transcriptionFiles []string
	for _, file := range files {
		if !file.IsDir() && filepath.Ext(file.Name()) == ".txt" {
			transcriptionFiles = append(transcriptionFiles, file.Name())
		}
	}

	// Sort files in descending order (newest first)
	sort.Slice(transcriptionFiles, func(i, j int) bool {
		return transcriptionFiles[i] > transcriptionFiles[j]
	})

	return transcriptionFiles
}

func initialModel(apiKey string) model {
	vp := viewport.New(0, 0)
	vp.Style = lipgloss.NewStyle().PaddingTop(1)
	h := help.New()

	return model{
		viewport:       vp,
		recordingState: Idle,
		senderStyle:    lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		err:           nil,
		help:          h,
		recorder:      audio.NewRecorder(),
		transcriber:   audio.NewTranscriber(apiKey),
		showCopied:    false,
		showingTranscriptions: false,
		transcriptionFiles: []string{},
		selectedIndex: 0,
		selectedContent: "",
	}
}

func (m model) Init() tea.Cmd {
	return textarea.Blink
}

func startRecording(recorder *audio.Recorder) tea.Cmd {
	return func() tea.Msg {
		err := recorder.StartRecording()
		if err != nil {
			return recordingStoppedMsg{err: err}
		}
		return recordingStartedMsg{}
	}
}

func stopRecording(recorder *audio.Recorder) tea.Cmd {
	return func() tea.Msg {
		if err := recorder.StopRecording(); err != nil {
			return recordingStoppedMsg{err: err}
		}
		return recordingStoppedMsg{err: nil}
	}
}

func transcribe(recorder *audio.Recorder, transcriber *audio.Transcriber) tea.Cmd {
	return func() tea.Msg {
		audioFile := recorder.GetOutputFile()
		text, err := transcriber.Transcribe(audioFile)
		if err != nil {
			return transcriptionFinishedMsg{err: err}
		}
		return transcriptionFinishedMsg{text: text}
	}
}

func tick() tea.Msg {
	time.Sleep(2 * time.Second)
	return tickMsg{}
}

func copyToClipboard(text string) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			return copyToClipboardMsg{err: err}
		}
		return copyToClipboardMsg{err: nil}
	}
}

func (m model) handleRecordingUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Record):
			if m.recordingState == Idle || m.recordingState == TranscriptionComplete {
				m.transcription = "" // Clear previous transcription when starting new recording
				return m, startRecording(m.recorder)
			}

		case key.Matches(msg, keys.StopRecording):
			if m.recordingState == Recording {
				return m, tea.Sequence(
					stopRecording(m.recorder),
					transcribe(m.recorder, m.transcriber),
				)
			}

		case key.Matches(msg, keys.CopyToClip):
			if m.transcription != "" && m.recordingState == TranscriptionComplete {
				return m, copyToClipboard(m.transcription)
			}
		}
	}

	return m, cmd
}

func (m model) transcriptionListView() string {
	if len(m.transcriptionFiles) == 0 {
		return paddedStyle.Render("No transcriptions found.\n\nPress ESC to go back")
	}

	if m.showingDeleteConfirmation {
		filename := m.transcriptionFiles[m.selectedIndex]
		audioFilename := strings.TrimSuffix(filename, ".txt") + ".wav"
		confirmMsg := fmt.Sprintf(
			"Are you sure you want to delete:\n• Transcription: %s\n• Audio: %s\n\nPress ENTER to confirm or ESC to cancel",
			filename,
			audioFilename,
		)
		return paddedStyle.Render(confirmMsg)
	}

	// Create two-pane view
	var leftPane strings.Builder
	leftPane.WriteString("Transcriptions:\n\n")
	
	// Calculate the width needed for the longest filename
	maxWidth := len("Transcriptions:") // minimum width
	for _, file := range m.transcriptionFiles {
		if len(file) > maxWidth {
			maxWidth = len(file)
		}
	}
	// Add padding for the prefix (2 chars) and some buffer space
	leftWidth := maxWidth + 6

	// If window is too narrow, only show the selected content
	const minWidthForSidebar = 100
	if m.width < minWidthForSidebar {
		if len(m.transcriptionFiles) > 0 {
			content := fmt.Sprintf("Selected Transcription (%d/%d):\n\n%s", 
				m.selectedIndex+1, 
				len(m.transcriptionFiles), 
				m.selectedContent,
			)
			if m.showCopied {
				content += "\n\n" + successStyle.Render("Copied to clipboard! ✓")
			}
			return paddedStyle.Render(content)
		}
		return paddedStyle.Render("No transcriptions found.\n\nPress ESC to go back")
	}
	
	for i, file := range m.transcriptionFiles {
		prefix := "  "
		if i == m.selectedIndex {
			prefix = "▶ "
		}
		// No need to truncate since we're using the natural width
		leftPane.WriteString(fmt.Sprintf("%s%s\n", prefix, file))
	}
	
	// Create right pane with selected content
	rightPane := fmt.Sprintf("Selected Transcription:\n\n%s", m.selectedContent)
	
	// Add copy confirmation if needed
	if m.showCopied {
		rightPane += "\n\n" + successStyle.Render("Copied to clipboard! ✓")
	}
	
	// Style the panes
	leftPaneStyled := lipgloss.NewStyle().
		Width(leftWidth).
		PaddingLeft(2).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderRight(true).
		Render(leftPane.String())
	
	rightPaneStyled := lipgloss.NewStyle().
		Width(m.width - leftWidth - 5). // Account for border and some padding
		PaddingLeft(2).
		Render(rightPane)
	
	return lipgloss.JoinHorizontal(lipgloss.Top, leftPaneStyled, rightPaneStyled)
}

func deleteTranscription(filename string) tea.Cmd {
	return func() tea.Msg {
		appDataDir, err := config.GetAppDataDir()
		if err != nil {
			return errMsg(fmt.Errorf("failed to get app data directory: %w", err))
		}

		transcriptionPath := filepath.Join(appDataDir, config.TranscriptionsDir, filename)
		if err := os.Remove(transcriptionPath); err != nil {
			return errMsg(fmt.Errorf("failed to delete transcription: %w", err))
		}

		// Also delete the corresponding audio file
		audioFilename := strings.TrimSuffix(filename, ".txt") + ".wav"
		audioPath := filepath.Join(appDataDir, config.RecordingsDir, audioFilename)
		_ = os.Remove(audioPath) // Ignore error as audio file might not exist

		return loadTranscriptions()
	}
}

func (m model) handleTranscriptionListUpdate(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// If showing delete confirmation, only handle Enter and Esc
		if m.showingDeleteConfirmation {
			switch {
			case key.Matches(msg, keys.Confirm):
				if len(m.transcriptionFiles) > 0 {
					m.showingDeleteConfirmation = false
					return m, deleteTranscription(m.transcriptionFiles[m.selectedIndex])
				}
			case key.Matches(msg, keys.Back):
				m.showingDeleteConfirmation = false
				m.viewport.SetContent(m.transcriptionListView())
			}
			return m, nil
		}

		// Normal key handling
		switch {
		case key.Matches(msg, keys.Delete):
			if len(m.transcriptionFiles) > 0 {
				m.showingDeleteConfirmation = true
				m.viewport.SetContent(m.transcriptionListView())
			}

		case key.Matches(msg, keys.Record):
			m.showingTranscriptions = false
			m.showCopied = false
			m.transcription = "" // Clear previous transcription
			m.recordingState = Idle // Ensure we're in Idle state
			m.viewport.SetContent(m.recordingView())
			return m, startRecording(m.recorder)

		case key.Matches(msg, keys.Up):
			if m.selectedIndex > 0 {
				m.selectedIndex--
				m.showCopied = false // Reset copy message when changing selection
				if content, err := loadTranscriptionContent(m.transcriptionFiles[m.selectedIndex]); err == nil {
					m.selectedContent = content
					m.viewport.SetContent(m.transcriptionListView())
				}
			}

		case key.Matches(msg, keys.Down):
			if m.selectedIndex < len(m.transcriptionFiles)-1 {
				m.selectedIndex++
				m.showCopied = false // Reset copy message when changing selection
				if content, err := loadTranscriptionContent(m.transcriptionFiles[m.selectedIndex]); err == nil {
					m.selectedContent = content
					m.viewport.SetContent(m.transcriptionListView())
				}
			}

		case key.Matches(msg, keys.CopyToClip):
			if m.selectedContent != "" {
				m.showCopied = false // Reset any previous copy message
				return m, copyToClipboard(m.selectedContent)
			}

		case key.Matches(msg, keys.Back):
			m.showingTranscriptions = false
			m.showCopied = false // Reset copy message when going back
			m.recordingState = Idle // Ensure we're in Idle state
			m.viewport.SetContent(m.recordingView())
		}
	}

	return m, cmd
}

func (m model) recordingView() string {
	var content string
	switch m.recordingState {
	case Recording:
		content = paddedStyle.Render("Recording... Press SPACE to stop")
	case Transcribing:
		content = paddedStyle.Render("Transcribing...")
	case Idle:
		if m.err != nil {
			content = paddedStyle.Render(fmt.Sprintf("Error: %v\nPress 'r' to start recording", m.err))
		} else {
			microphone := ""
			if m.width >= 30 && m.height >= 25 {
				microphone = smallMicrophone
			}
			content = paddedStyle.Render(fmt.Sprintf("%sPress 'r' to start recording", microphone))
		}
	case TranscriptionComplete:
		mainContent := fmt.Sprintf("Transcription complete:\n\n%s", m.transcription)
		if m.showCopied {
			content = fmt.Sprintf(
				"%s\n\n%s",
				paddedStyle.Render(mainContent),
				successStyleWithPadding.Render("Copied to clipboard! ✓"),
			)
		} else {
			content = paddedStyle.Render(mainContent)
		}
	}
	return content
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Calculate the height available for content
		helpHeight := lipgloss.Height(m.help.View(m))
		verticalMarginHeight := 2 // top and bottom margins

		// Set viewport dimensions
		m.viewport.Width = msg.Width
		m.viewport.Height = msg.Height - helpHeight - verticalMarginHeight
		m.width = msg.Width
		m.height = msg.Height

		// Update content based on current view
		if m.showingTranscriptions {
			m.viewport.SetContent(m.transcriptionListView())
		} else {
			m.viewport.SetContent(m.recordingView())
		}

	case recordingStartedMsg:
		m.recordingState = Recording
		m.err = nil

	case recordingStoppedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.recordingState = Idle
		} else {
			m.recordingState = Transcribing
		}

	case transcriptionFinishedMsg:
		m.recordingState = TranscriptionComplete
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.transcription = msg.text
			// Reload transcription files after successful transcription
			if m.showingTranscriptions {
				return m, loadTranscriptions
			}
		}

	case copyToClipboardMsg:
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.showCopied = true
			return m, tea.Batch(cmd, tea.Sequence(tick))
		}

	case tickMsg:
		m.showCopied = false

	case []string:
		m.transcriptionFiles = msg
		if len(m.transcriptionFiles) > 0 {
			if content, err := loadTranscriptionContent(m.transcriptionFiles[0]); err == nil {
				m.selectedContent = content
			}
		}
		// Update viewport content immediately after loading files
		m.viewport.SetContent(m.transcriptionListView())
		return m, nil

	case errMsg:
		m.err = msg
		return m, nil

	case tea.KeyMsg:
		// Global key handlers
		switch {
		case key.Matches(msg, keys.Back) && m.help.ShowAll:
			m.help.ShowAll = false
			// Trigger a window resize to recalculate viewport height
			return m, func() tea.Msg {
				return tea.WindowSizeMsg{
					Width:  m.width,
					Height: m.height,
				}
			}

		case key.Matches(msg, keys.Help):
			m.help.ShowAll = !m.help.ShowAll
			// Trigger a window resize to recalculate viewport height
			return m, func() tea.Msg {
				return tea.WindowSizeMsg{
					Width:  m.width,
					Height: m.height,
				}
			}

		case key.Matches(msg, keys.Quit):
			return m, tea.Quit

		case key.Matches(msg, keys.ListTranscriptions):
			m.showingTranscriptions = !m.showingTranscriptions
			if m.showingTranscriptions {
				m.selectedIndex = 0
				m.selectedContent = ""
				content := paddedStyle.Render("Loading transcriptions...\n\nPress ESC to go back")
				m.viewport.SetContent(content)
				return m, loadTranscriptions
			}
			return m, nil
		}

		// If help is showing, don't process other keys
		if m.help.ShowAll {
			return m, nil
		}

		// Route to the appropriate update handler based on current view
		if m.showingTranscriptions {
			return m.handleTranscriptionListUpdate(msg)
		} else {
			return m.handleRecordingUpdate(msg)
		}
	}

	// Set the viewport content based on current view
	if m.showingTranscriptions {
		m.viewport.SetContent(m.transcriptionListView())
	} else {
		m.viewport.SetContent(m.recordingView())
	}

	if len(cmds) > 0 {
		return m, tea.Batch(cmds...)
	}
	return m, cmd
}

func (m model) ShortHelp() []key.Binding {
	if m.showingTranscriptions {
		if m.showingDeleteConfirmation {
			return []key.Binding{
				keys.Confirm,
				keys.Back,
				keys.Help,
			}
		}
		if m.width < 100 {
			return []key.Binding{
				keys.Up,
				keys.Down,
				keys.CopyToClip,
				keys.Help,
			}
		} else {
			return []key.Binding{
				keys.CopyToClip,
				keys.Delete,
				keys.Help,
			}
		}
	}

	switch m.recordingState {
	case Idle:
		return []key.Binding{
			keys.Record,
			keys.ListTranscriptions,
			keys.Help,
		}
	case Recording:
		return []key.Binding{
			keys.StopRecording,
			keys.Help,
		}
	case Transcribing:
		return []key.Binding{
			keys.Help,
		}
	case TranscriptionComplete:
		return []key.Binding{
			keys.Record,
			keys.CopyToClip,
			keys.ListTranscriptions,
			keys.Help,
		}
	default:
		panic(fmt.Sprintf("unhandled recording state: %v", m.recordingState))
	}
}

func (m model) FullHelp() [][]key.Binding {
	if m.showingTranscriptions {
		if m.showingDeleteConfirmation {
			return [][]key.Binding{
				{keys.Confirm, keys.Back}, // Confirmation actions
				{keys.Help, keys.Quit},    // Global controls
			}
		}
		return [][]key.Binding{
			{keys.Up, keys.Down, keys.Back, keys.CopyToClip, keys.Delete}, // Navigation and actions
			{keys.Help, keys.Quit},                  // Global controls
		}
	}

	switch m.recordingState {
	case Recording:
		return [][]key.Binding{
			{keys.StopRecording},        // first column
			{keys.Help, keys.Quit},      // second column
			{key.NewBinding(key.WithHelp("Note", "Recording will automatically stop after 20 minutes"))},
		}
	case TranscriptionComplete:
		return [][]key.Binding{
			{keys.Record, keys.CopyToClip, keys.ListTranscriptions}, // first column
			{keys.Help, keys.Quit},                                  // second column
		}
	default:
		return [][]key.Binding{
			{keys.Record, keys.ListTranscriptions}, // first column
			{keys.Help, keys.Quit},                // second column
			{key.NewBinding(key.WithHelp("Note", "Recording will automatically stop after 20 minutes"))},
		}
	}
}

func (m model) View() string {
	var b strings.Builder

	// Add top margin
	b.WriteString("\n")
	
	// Add viewport content
	b.WriteString(m.viewport.View())
	
	// Add bottom margin and help
	b.WriteString("\n")

	// Add warning if help is shown and we're in recording or idle state
	if (m.help.ShowAll) {
		b.WriteString(helpStyle.Render("Note: Recordings automatically stop after 20 minutes"))
		b.WriteString("\n")
	}
	
	b.WriteString(helpStyle.Render(m.help.View(m)))

	return b.String()
}
