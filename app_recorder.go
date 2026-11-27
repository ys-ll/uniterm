package main

import (
	"fmt"

	"github.com/ys-ll/uniterm/backend/recorder"
)

// app_recorder.go exposes the terminal recorder (asciinema cast v2) as Wails
// bindings. The frontend terminalRecorder.ts service calls these to start
// recording a session, stream session:data into the cast file, and later
// list/delete/play back recordings.
//
// All methods delegate to the process-wide recorder.Recorder singleton
// (recorder.New()), so no App field is required. The singleton pattern keeps
// the hot data path (RecorderWriteData) lock-free at the App layer.

// recorderInst returns the shared Recorder. Constructed lazily on first call
// so apps that never record pay zero cost.
func recorderInst() *recorder.Recorder {
	return recorder.New()
}

// RecorderStart creates a new cast v2 file under <home>/Documents/uniTerm/records/
// and writes the JSON header. Returns the absolute file path the frontend should
// pass to subsequent RecorderWriteData / RecorderStop calls.
func (a *App) RecorderStart(sessionID string, width int, height int, title string) (string, error) {
	return recorderInst().StartRecording(sessionID, width, height, title)
}

// RecorderWriteData appends one `[elapsed, "o", data]` row to the cast file
// at filePath. Called from the terminal session:data hook in BaseTerminal.vue.
// No-op (returns nil) if the path is not an open recording — the frontend
// data path must not error when a recording was stopped concurrently.
func (a *App) RecorderWriteData(filePath string, data string) error {
	filePath, err := a.confineLocalPath(filePath)
	if err != nil {
		return err
	}
	return recorderInst().WriteRecordingData(filePath, data)
}

// RecorderStop flushes and closes the recording at filePath. Idempotent.
func (a *App) RecorderStop(filePath string) error {
	filePath, err := a.confineLocalPath(filePath)
	if err != nil {
		return err
	}
	return recorderInst().StopRecording(filePath)
}

// RecorderIsRecording reports whether filePath is currently being recorded.
func (a *App) RecorderIsRecording(filePath string) bool {
	return recorderInst().IsRecording(filePath)
}

// RecorderList returns metadata for every .cast file in the records
// directory, newest first.
func (a *App) RecorderList() ([]recorder.RecordingInfo, error) {
	return recorderInst().ListRecordings()
}

// RecorderDelete removes a cast file from disk. If the recording is open it
// is stopped first.
func (a *App) RecorderDelete(filePath string) error {
	filePath, err := a.confineLocalPath(filePath)
	if err != nil {
		return err
	}
	return recorderInst().DeleteRecording(filePath)
}

// RecorderGetContent reads the entire cast file as a string. The frontend
// player parses the header + data rows to drive xterm.js replay.
func (a *App) RecorderGetContent(filePath string) (string, error) {
	filePath, err := a.confineLocalPath(filePath)
	if err != nil {
		return "", err
	}
	return recorderInst().GetRecordingContent(filePath)
}

// RecorderOpenDir opens the records directory in the OS file manager.
func (a *App) RecorderOpenDir() error {
	dir := recorderInst().RecordsDir()
	if dir == "" {
		return fmt.Errorf("recorder: records directory unavailable")
	}
	return a.OpenPathInExplorer(dir)
}
