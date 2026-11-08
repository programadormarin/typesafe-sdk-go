package typesafe

import (
	"bytes"
	"encoding/hex"
	crand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

const (
	jsonContentType  = "application/json"
	referencePrefix  = "<image:"
)

var referencePattern = regexp.MustCompile(`<image:[0-9a-f]{32}>`)

type attachedImage struct {
	image *Image
	holds int
}

func (c *Client) AttachAssets(images ...*Image) (release func()) {
	images = slices.Clone(images)
	for _, image := range images {
		if image == nil {
			panic("typesafe: AttachAssets called with a nil *Image")
		}
	}
	c.attachedMu.Lock()
	defer c.attachedMu.Unlock()
	for _, image := range images {
		entry := c.attached[image.reference]
		if entry == nil {
			entry = &attachedImage{image: image}
			c.attached[image.reference] = entry
		}
		entry.holds++
	}
	return sync.OnceFunc(func() {
		c.attachedMu.Lock()
		defer c.attachedMu.Unlock()
		for _, image := range images {
			entry := c.attached[image.reference]
			if entry.holds--; entry.holds == 0 {
				delete(c.attached, image.reference)
			}
		}
	})
}

func (c *Client) attachedImages() map[string]*Image {
	c.attachedMu.Lock()
	defer c.attachedMu.Unlock()
	images := make(map[string]*Image, len(c.attached))
	for reference, entry := range c.attached {
		images[reference] = entry.image
	}
	return images
}

type pendingAsset struct {
	name  string
	image *Image
}

func (c *Client) withAssets(encoded []byte, request SystemOneRequest) ([]byte, string, error) {
	if _, ok := request.ExtraBody["assets"]; ok {
		return encoded, jsonContentType, nil
	}
	known := c.attachedImages()
	pending, aliases, err := passedAssets(request.Assets, known)
	if err != nil {
		return nil, "", err
	}
	placed := map[string]*Image{}
	collectImages(placed, map[visit]bool{}, reflect.ValueOf(request.State))
	collectImages(placed, map[visit]bool{}, reflect.ValueOf(request.Questions))
	collectImages(placed, map[visit]bool{}, reflect.ValueOf(request.ExtraBody))
	for reference, image := range placed {
		known[reference] = image
	}
	if len(known) == 0 {
		return encoded, jsonContentType, nil
	}
	rewritten, found, err := scanReferences(encoded, known, aliases)
	if err != nil {
		return nil, "", err
	}
	seen := make(map[string]bool, len(pending))
	for _, asset := range pending {
		seen[asset.name] = true
	}
	for _, reference := range found {
		if !seen[reference] {
			seen[reference] = true
			pending = append(pending, pendingAsset{name: reference, image: known[reference]})
		}
	}
	if len(pending) == 0 {
		return encoded, jsonContentType, nil
	}
	resolved := make([]*resolvedImage, len(pending))
	for i, asset := range pending {
		if resolved[i], err = asset.image.resolve(); err != nil {
			return nil, "", fmt.Errorf("Asset %q: %w", asset.name, err)
		}
	}
	body, contentType := encodeMultipart(spliceAssets(rewritten, pending), resolved)
	return body, contentType, nil
}

func passedAssets(assets map[string]*Image, known map[string]*Image) (pending []pendingAsset, aliases map[string]string, err error) {
	names := make([]string, 0, len(assets))
	for name := range assets {
		names = append(names, name)
	}
	slices.Sort(names)
	aliases = make(map[string]string, len(names))
	for _, name := range names {
		image := assets[name]
		if strings.TrimSpace(name) == "" {
			return nil, nil, &requestError{msg: "Asset aliases must be non-empty strings."}
		}
		if image == nil {
			return nil, nil, &requestError{msg: fmt.Sprintf("Asset %q must be an asset.", name)}
		}
		pending = append(pending, pendingAsset{name: name, image: image})
		known[image.reference] = image
		aliases[image.reference] = name
	}
	return pending, aliases, nil
}

func spliceAssets(body []byte, pending []pendingAsset) []byte {
	spliced := append([]byte(nil), body[:len(body)-1]...)
	spliced = append(spliced, `,"assets":{`...)
	for i, asset := range pending {
		if i > 0 {
			spliced = append(spliced, ',')
		}
		name, _ := json.Marshal(asset.name)
		spliced = append(spliced, name...)
		spliced = append(spliced, `:{"type":"image","part":`...)
		spliced = strconv.AppendQuote(spliced, partName(i))
		spliced = append(spliced, '}')
	}
	return append(spliced, "}}"...)
}

func partName(index int) string { return "asset-" + strconv.Itoa(index) }

func encodeMultipart(body []byte, images []*resolvedImage) ([]byte, string) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	var boundary [16]byte
	_, _ = crand.Read(boundary[:])
	_ = writer.SetBoundary(hex.EncodeToString(boundary[:]))
	parts := make([]struct {
		header  textproto.MIMEHeader
		content []byte
	}, 0, len(images)+1)
	parts = append(parts, struct {
		header  textproto.MIMEHeader
		content []byte
	}{
		header:  textproto.MIMEHeader{"Content-Disposition": {`form-data; name="request"`}, "Content-Type": {jsonContentType}},
		content: body,
	})
	for i, image := range images {
		name := partName(i)
		parts = append(parts, struct {
			header  textproto.MIMEHeader
			content []byte
		}{
			header: textproto.MIMEHeader{
				"Content-Disposition": {fmt.Sprintf(`form-data; name=%q; filename=%q`, name, name)},
				"Content-Type":        {image.contentType},
			},
			content: image.data,
		})
	}
	for _, part := range parts {
		partWriter, _ := writer.CreatePart(part.header)
		_, _ = partWriter.Write(part.content)
	}
	_ = writer.Close()
	return buffer.Bytes(), writer.FormDataContentType()
}

func scanReferences(encoded []byte, known map[string]*Image, aliases map[string]string) (rewritten []byte, found []string, err error) {
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var out bytes.Buffer
	copied := 0
	var open []container
	seen := map[string]bool{}
	for {
		previous := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, nil, fmt.Errorf("scan request body: %w", err)
		}
		if delim, ok := token.(json.Delim); ok {
			if delim == '{' || delim == '[' {
				open = append(open, container{object: delim == '{', key: delim == '{'})
				continue
			}
			open = open[:len(open)-1]
		}
		isKey := false
		if n := len(open); n > 0 && open[n-1].object {
			isKey = open[n-1].key
			open[n-1].key = !isKey
		}
		text, ok := token.(string)
		if !ok || !strings.Contains(text, referencePrefix) {
			continue
		}
		if alias, ok := aliases[text]; ok && !isKey {
			start := int(previous) + bytes.IndexByte(encoded[previous:], '"')
			replacement, _ := json.Marshal(alias)
			out.Write(encoded[copied:start])
			out.Write(replacement)
			copied = int(decoder.InputOffset())
			continue
		}
		for _, reference := range referencePattern.FindAllString(text, -1) {
			if _, ok := known[reference]; ok && !seen[reference] {
				seen[reference] = true
				found = append(found, reference)
			}
		}
	}
	if len(open) > 0 {
		return nil, nil, fmt.Errorf("scan request body: %w", io.ErrUnexpectedEOF)
	}
	out.Write(encoded[copied:])
	return out.Bytes(), found, nil
}

type container struct {
	object bool
	key    bool
}

type visit struct {
	pointer uintptr
	typ     reflect.Type
	length  int
}

func collectImages(images map[string]*Image, visited map[visit]bool, v reflect.Value) {
	if !v.IsValid() {
		return
	}
	if v.Type() == imageType {
		if !v.IsNil() {
			image := v.Interface().(*Image)
			images[image.reference] = image
		}
		return
	}
	switch v.Kind() {
	case reflect.Interface:
		collectImages(images, visited, v.Elem())
	case reflect.Pointer:
		if v.IsNil() || !markVisited(visited, v) {
			return
		}
		collectImages(images, visited, v.Elem())
	case reflect.Map:
		if v.IsNil() || !markVisited(visited, v) {
			return
		}
		for iter := v.MapRange(); iter.Next(); {
			collectImages(images, visited, iter.Value())
		}
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && (v.IsNil() || !markVisited(visited, v)) {
			// still allow arrays
			if v.Kind() == reflect.Slice {
				return
			}
		}
		for i := 0; i < v.Len(); i++ {
			collectImages(images, visited, v.Index(i))
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if field.Tag.Get("json") == "-" {
				continue
			}
			if !field.IsExported() && !(field.Anonymous && field.Type.Kind() == reflect.Struct) {
				continue
			}
			collectImages(images, visited, v.Field(i))
		}
	}
}

func markVisited(visited map[visit]bool, v reflect.Value) bool {
	key := visit{pointer: v.Pointer(), typ: v.Type()}
	if v.Kind() == reflect.Slice {
		key.length = v.Len()
	}
	if visited[key] {
		return false
	}
	visited[key] = true
	return true
}

// adjust collectImages slice handling condition: if array, loop without mark. Already loops even if mark not called? For arrays markVisited not called because condition only slices; but we returned only for slices. Good.
