# Elevator Pitch
🚨 New openai / ffmpeg wrapper just dropped 🚨  

Easily transcribe your voice directly from your favorite terminal. Lazywhisper is great for vibe coding or just getting your thoughts down.

# Features
- Intuitive keyboard controls
  - Vim keybindings 
  - `r` to record, `y/c` to copy transcription
- Run from any terminal 
  - Integrated Cursor Terminal / Ghostty / etc
- View old transcriptions

# Installation & Setup
## Pre-reqs
- Install go if you haven't
  - https://go.dev/doc/install
- Install ffmpeg if you haven't 
  - https://www.ffmpeg.org/download.html
- Get an OpenAI Api Key and export it as an enviornment variable in you shell e.g. `~/.zshrc` or `~/.bashrc`
  - https://platform.openai.com/docs/libraries#create-and-export-an-api-key
  - `export OPENAI_API_KEY="your_api_key_here"`

## Install
- Clone this repo: `git clone git@github.com:bensenescu/lazywhisper.git`
- Navigate to the directory: `cd lazywhisper`
- Compile the app: `go build` 
- Move the program to your path
  - On Mac: `sudo mv lazywhisper /usr/local/bin`
- Test that it worked: `lazywhisper`
  - Note: You may need to give your terminal permission to use your microphone.

# Usage
- `r` - Record a transcription
- `y/c` - Copy your transcription
- `l` - List old transcriptions
- `d` - Delete transcription
