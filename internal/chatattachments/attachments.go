package chatattachments

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const MaxAttachmentBytes = 20 << 20

type Attachment struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Path     string `json:"path"`
	MIMEType string `json:"mimeType,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

type Input struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name,omitempty"`
	Path     string `json:"path,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	Size     int64  `json:"size,omitempty"`
	Data     string `json:"data,omitempty"`
}

func FromPath(path string) (Attachment, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Attachment{}, errors.New("attachment path required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Attachment{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Attachment{}, err
	}
	if info.IsDir() {
		return Attachment{}, fmt.Errorf("attachment is a directory: %s", abs)
	}
	if !allowedImage(abs, "") {
		return Attachment{}, fmt.Errorf("unsupported image type: %s", abs)
	}
	return Attachment{
		ID:       "att-" + shortID(),
		Name:     filepath.Base(abs),
		Path:     abs,
		MIMEType: mime.TypeByExtension(strings.ToLower(filepath.Ext(abs))),
		Size:     info.Size(),
	}, nil
}

func Resolve(sessionID string, inputs []Input) ([]Attachment, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	attachments := make([]Attachment, 0, len(inputs))
	for _, input := range inputs {
		switch {
		case strings.TrimSpace(input.Path) != "":
			attachment, err := FromPath(input.Path)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(input.ID) != "" {
				attachment.ID = input.ID
			}
			if strings.TrimSpace(input.Name) != "" {
				attachment.Name = sanitizeFilename(input.Name)
			}
			attachments = append(attachments, attachment)
		case strings.TrimSpace(input.Data) != "":
			attachment, err := SaveData(sessionID, input)
			if err != nil {
				return nil, err
			}
			attachments = append(attachments, attachment)
		default:
			return nil, errors.New("attachment requires path or data")
		}
	}
	return attachments, nil
}

func SaveData(sessionID string, input Input) (Attachment, error) {
	dataText := strings.TrimSpace(input.Data)
	if dataText == "" {
		return Attachment{}, errors.New("attachment data required")
	}
	if idx := strings.Index(dataText, ","); strings.HasPrefix(dataText, "data:") && idx >= 0 {
		if input.MIMEType == "" {
			header := strings.TrimPrefix(dataText[:idx], "data:")
			input.MIMEType = strings.TrimSuffix(header, ";base64")
		}
		dataText = dataText[idx+1:]
	}
	data, err := base64.StdEncoding.DecodeString(dataText)
	if err != nil {
		return Attachment{}, fmt.Errorf("decode attachment: %w", err)
	}
	if len(data) == 0 {
		return Attachment{}, errors.New("attachment is empty")
	}
	if len(data) > MaxAttachmentBytes {
		return Attachment{}, fmt.Errorf("attachment is too large: %d bytes", len(data))
	}

	name := sanitizeFilename(input.Name)
	if name == "" {
		name = "image"
	}
	if filepath.Ext(name) == "" {
		name += extensionForMIME(input.MIMEType)
	}
	if !allowedImage(name, input.MIMEType) {
		return Attachment{}, fmt.Errorf("unsupported image type: %s", name)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return Attachment{}, err
	}
	sessionDir := sanitizeFilename(sessionID)
	if sessionDir == "" {
		sessionDir = "session"
	}
	dir := filepath.Join(home, ".orion", "attachments", sessionDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Attachment{}, err
	}

	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = "att-" + shortID()
	}
	filename := time.Now().UTC().Format("20060102-150405") + "-" + shortID() + "-" + name
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return Attachment{}, err
	}

	return Attachment{
		ID:       id,
		Name:     name,
		Path:     path,
		MIMEType: input.MIMEType,
		Size:     int64(len(data)),
	}, nil
}

func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		switch r {
		case '.', '-', '_':
			return r
		default:
			return '-'
		}
	}, name)
	name = strings.Trim(name, ".-")
	if len(name) > 120 {
		ext := filepath.Ext(name)
		base := strings.TrimSuffix(name, ext)
		if len(base) > 100 {
			base = base[:100]
		}
		name = base + ext
	}
	return name
}

func allowedImage(path string, mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".heic", ".heif":
		return true
	default:
		return false
	}
}

func extensionForMIME(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0])) {
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "image/heic":
		return ".heic"
	case "image/heif":
		return ".heif"
	default:
		return ".jpg"
	}
}

func shortID() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b[:])
}
