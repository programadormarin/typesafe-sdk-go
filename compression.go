package typesafe

import (
	"fmt"

	"github.com/klauspost/compress/zstd"
)

// Compression selects how request bodies are compressed.
type Compression string

const (
	CompressionDefault Compression = ""
	CompressionZstd    Compression = "zstd"
	CompressionNone    Compression = "none"
)

func (c Compression) validate() error {
	switch c {
	case CompressionDefault, CompressionZstd, CompressionNone:
		return nil
	}
	return fmt.Errorf("compression must be %q, %q, or empty: %w", CompressionZstd, CompressionNone, ErrInvalidRequest)
}

const minCompressedBodyLength = 1024

var zstdEnc = func() *zstd.Encoder {
	enc, _ := zstd.NewWriter(nil)
	return enc
}()

func zstdCompress(body []byte) []byte {
	return zstdEnc.EncodeAll(body, nil)
}
