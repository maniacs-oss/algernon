package main

import (
	"bytes"
	"fmt"
	log "github.com/sirupsen/logrus"
	"github.com/yuin/gopher-lua"
	"io"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
)

// For dealing with uploaded files in POST method handlers

const (
	// Identifier for the UploadedFile class in Lua
	lUploadedFileClass = "UploadedFile"

	// Upload limit, in bytes
	defaultUploadLimit int64 = 32 * MiB

	// Memory usage while uploading
	defaultMemoryLimit int64 = 32 * MiB

	// Chunk size when reading uploaded file
	//chunkSize int64 = 4 * KiB
	chunkSize = defaultMemoryLimit
)

// UploadedFile represents a file that has been uploaded but not yet been
// written to file.
type UploadedFile struct {
	req       *http.Request
	scriptdir string
	header    textproto.MIMEHeader
	filename  string
	buf       *bytes.Buffer
}

// Receives an uploadeded file
//
// The client will send all the data, if the data is over the given size,
// if the Content-Length is wrongly set to a value below the the uploadLimit.
// However, the buffer and memory usage will not grow despite this.
//
// uploadLimit is in bytes.
//
// Note that the client may appear to keep sending the file even when the
// server has stopped receiving it, for files that are too large.
func newUploadedFile(req *http.Request, scriptdir, formID string, uploadLimit int64) (*UploadedFile, error) {

	clientLengthTotal, err := strconv.Atoi(req.Header.Get("Content-Length"))
	if err != nil {
		log.Error("Invalid Content-Length: ", req.Header.Get("Content-Length"))
	}
	// Remove the extra 20 bytes and convert to int64
	clientLength := int64(clientLengthTotal - 20)

	if clientLength > uploadLimit {
		return nil, fmt.Errorf("Uploaded file was too large: %s according to Content-Length (current limit is %s)", describeBytes(clientLength), describeBytes(uploadLimit))
	}

	// For specifying the memory usage when uploading
	if err := req.ParseMultipartForm(defaultMemoryLimit); err != nil {
		return nil, err
	}
	file, handler, err := req.FormFile(formID)
	defer file.Close()
	if err != nil {
		return nil, err
	}

	// Store the data in a buffer, for later usage.
	// The buffer has a limited size.
	data := make([]byte, uploadLimit)
	buf := bytes.NewBuffer(data)

	// Read the data in chunks
	var totalWritten, writtenBytes, i int64
	for i = 0; i < int64(buf.Len()); i += chunkSize {
		writtenBytes, err = io.CopyN(buf, file, chunkSize)
		totalWritten += writtenBytes
		if totalWritten > uploadLimit {
			// File too large
			err := fmt.Errorf("Uploaded file was too large: %d bytes (limit is %d bytes)", totalWritten, uploadLimit)
			return nil, err
		} else if writtenBytes < chunkSize || err == io.EOF {
			// Done writing
			break
		} else if err != nil {
			// Error when copying data
			return nil, err
		}
	}

	// trunkate the buffer to the uploaded size
	buf.Truncate(int(totalWritten))

	// all ok
	return &UploadedFile{req, scriptdir, handler.Header, handler.Filename, buf}, nil
}

// Get the first argument, "self", and cast it from userdata to
// an UploadedFile, which contains the file data and information.
func checkUploadedFile(L *lua.LState) *UploadedFile {
	ud := L.CheckUserData(1)
	if uploadedfile, ok := ud.Value.(*UploadedFile); ok {
		return uploadedfile
	}
	L.ArgError(1, "UploadedFile expected")
	return nil
}

// Create a new Upload file
func constructUploadedFile(L *lua.LState, req *http.Request, scriptdir, formID string, uploadLimit int64) (*lua.LUserData, error) {
	// Create a new UploadedFile
	uploadedfile, err := newUploadedFile(req, scriptdir, formID, uploadLimit)
	if err != nil {
		return nil, err
	}
	// Create a new userdata struct
	ud := L.NewUserData()
	ud.Value = uploadedfile
	L.SetMetatable(ud, L.GetTypeMetatable(lUploadedFileClass))
	return ud, nil
}

// String representation
func uploadedfileToString(L *lua.LState) int {
	L.Push(lua.LString("Uploaded file"))
	return 1 // number of results
}

// File name
func uploadedfileName(L *lua.LState) int {
	ulf := checkUploadedFile(L) // arg 1
	L.Push(lua.LString(ulf.filename))
	return 1 // number of results
}

// File size
func uploadedfileSize(L *lua.LState) int {
	ulf := checkUploadedFile(L) // arg 1
	L.Push(lua.LNumber(ulf.buf.Len()))
	return 1 // number of results
}

// Mime type
func uploadedfileMimeType(L *lua.LState) int {
	ulf := checkUploadedFile(L) // arg 1
	contentType := ""
	if contentTypes, ok := ulf.header["Content-Type"]; ok {
		if len(contentTypes) > 0 {
			contentType = contentTypes[0]
		}
	}
	L.Push(lua.LString(contentType))
	return 1 // number of results
}

// Write the uploaded file to the given full filename.
// Does not overwrite files.
func (ulf *UploadedFile) write(fullFilename string) error {
	// Check if the file already exists
	if _, err := os.Stat(fullFilename); err == nil { // exists
		log.Error(fullFilename, " already exists")
		return fmt.Errorf("File exists: " + fullFilename)
	}
	// Write the uploaded file
	f, err := os.OpenFile(fullFilename, os.O_WRONLY|os.O_CREATE, defaultPermissions)
	defer f.Close()
	if err != nil {
		log.Error("Error when creating ", fullFilename)
		return err
	}
	if _, err := io.Copy(f, ulf.buf); err != nil {
		log.Error("Error when writing: " + err.Error())
		return err
	}
	return nil
}

// Save the file locally
func uploadedfileSave(L *lua.LState) int {
	ulf := checkUploadedFile(L) // arg 1
	givenFilename := ""
	if L.GetTop() == 2 {
		givenFilename = L.ToString(2) // optional argument
	}

	// Use the given filename instead of the default one, if given
	var filename string
	if givenFilename != "" {
		filename = givenFilename
	} else {
		filename = ulf.filename
	}

	// Get the full path
	writeFilename := filepath.Join(ulf.scriptdir, filename)

	// Write the file and return true if successful
	L.Push(lua.LBool(ulf.write(writeFilename) == nil))
	return 1 // number of results
}

// Save the file locally, to a given directory
func uploadedfileSaveIn(L *lua.LState) int {
	ulf := checkUploadedFile(L)     // arg 1
	givenDirectory := L.ToString(2) // required argument

	// Get the full path
	var writeFilename string
	if filepath.IsAbs(givenDirectory) {
		writeFilename = filepath.Join(givenDirectory, ulf.filename)
	} else {
		writeFilename = filepath.Join(ulf.scriptdir, givenDirectory, ulf.filename)
	}

	// Write the file and return true if successful
	L.Push(lua.LBool(ulf.write(writeFilename) == nil))
	return 1 // number of results
}

// The hash map methods that are to be registered
var uploadedfileMethods = map[string]lua.LGFunction{
	"__tostring": uploadedfileToString,
	"filename":   uploadedfileName,
	"size":       uploadedfileSize,
	"mimetype":   uploadedfileMimeType,
	"save":       uploadedfileSave,
	"savein":     uploadedfileSaveIn,
}

// Make functions related to saving an uploaded file available
func exportUploadedFile(L *lua.LState, w http.ResponseWriter, req *http.Request, scriptdir string) {

	// Register the UploadedFile class and the methods that belongs with it.
	mt := L.NewTypeMetatable(lUploadedFileClass)
	mt.RawSetH(lua.LString("__index"), mt)
	L.SetFuncs(mt, uploadedfileMethods)

	// The constructor for the UploadedFile userdata
	// Takes a form ID (string) and an optional file upload limit in MiB
	// (number). Returns the userdata and an empty string on success.
	// Returns nil and an error message on failure.
	L.SetGlobal("UploadedFile", L.NewFunction(func(L *lua.LState) int {
		formID := L.ToString(1)
		if formID == "" {
			L.ArgError(1, "form ID expected")
		}
		uploadLimit := defaultUploadLimit
		if L.GetTop() == 2 {
			uploadLimit = int64(L.ToInt(2)) * MiB // optional upload limit, in MiB
		}
		// Construct a new UploadedFile
		userdata, err := constructUploadedFile(L, req, scriptdir, formID, uploadLimit)
		if err != nil {
			// Log the error
			log.Error(err)

			// Return an invalid UploadedFile object and an error string.
			// It's up to the Lua script to send an error to the client.
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2 // Number of returned values
		}

		// Return the Lua UploadedFile object and an empty error string
		L.Push(userdata)
		L.Push(lua.LString(""))
		return 2 // Number of returned values
	}))

}