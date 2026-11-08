package typesafe

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"sync"

	"reflect"
)

var readFile = os.ReadFile

const (
	contentTypePNG     = "image/png"
	contentTypeJPEG    = "image/jpeg"
	contentTypeUnknown = "application/octet-stream"
)

var (
	pngSignature  = []byte("\x89PNG\r\n\x1a\n")
	jpegSignature = []byte("\xff\xd8\xff")
)

type requestError struct {
	msg   string
	cause error
}

func (e *requestError) Error() string { return e.msg }

func (e *requestError) Unwrap() []error {
	if e.cause == nil {
		return []error{ErrInvalidRequest}
	}
	return []error{ErrInvalidRequest, e.cause}
}

type Image struct {
	reference string
	data      []byte
	path      string
	fromFile  bool

	mu       sync.Mutex
	resolved *resolvedImage
}

type resolvedImage struct {
	contentType string
	data        []byte
}

func newReference() string {
	var id [16]byte
	_, _ = rand.Read(id[:])
	return "<image:" + hex.EncodeToString(id[:]) + ">"
}

func NewImage(data []byte) *Image {
	return &Image{reference: newReference(), data: data}
}

func ImageFromFile(path string) *Image {
	return &Image{reference: newReference(), path: path, fromFile: true}
}

func ImageFromBase64(data string) (*Image, error) {
	var text strings.Builder
	for i := 0; i < len(data); i++ {
		c := data[i]
		switch {
		case c >= 0x80:
			return nil, &requestError{msg: "Image data must contain non-empty ASCII base64 text."}
		case c == ' ' || (c >= '\t' && c <= '\r'):
		default:
			text.WriteByte(c)
		}
	}
	if text.Len() == 0 {
		return nil, &requestError{msg: "Image data must contain non-empty ASCII base64 text."}
	}
	decoded, err := base64.StdEncoding.DecodeString(text.String())
	if err != nil {
		return nil, &requestError{msg: "Image data must contain valid base64 text.", cause: err}
	}
	return NewImage(decoded), nil
}

func (i *Image) String() string { return i.reference }

func (i *Image) MarshalJSON() ([]byte, error) { return json.Marshal(i.reference) }

func (i *Image) resolve() (*resolvedImage, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.resolved != nil {
		return i.resolved, nil
	}
	data := i.data
	if i.fromFile {
		var err error
		if data, err = readFile(i.path); err != nil {
			reason := err.Error()
			var pathErr *fs.PathError
			if errors.As(err, &pathErr) {
				reason = pathErr.Err.Error()
			}
			return nil, &requestError{msg: fmt.Sprintf("Image file %q could not be read: %s.", i.path, reason), cause: err}
		}
	} else {
		data = bytes.Clone(data)
	}
	if len(data) == 0 {
		return nil, &requestError{msg: "Image data is empty."}
	}
	i.resolved = &resolvedImage{contentType: contentType(data), data: data}
	return i.resolved, nil
}

func contentType(data []byte) string {
	switch {
	case bytes.HasPrefix(data, pngSignature):
		return contentTypePNG
	case bytes.HasPrefix(data, jpegSignature):
		return contentTypeJPEG
	}
	return contentTypeUnknown
}

// keep reflect imported usage? unused if we don't reference reflect. Use blank? imageType var needs reflect; add.
var imageType = reflect.TypeFor[*Image]()
