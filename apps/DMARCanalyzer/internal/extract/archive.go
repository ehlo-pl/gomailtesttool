// Package extract pulls DMARC aggregate report XML documents out of e-mail
// messages (.eml) and loose report files (.xml, .xml.gz, .gz, .zip).
//
// All processing happens in memory; archive entry names are never used as
// file system paths. Size limits guard against decompression bombs.
package extract

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

// Limits bounds the work done on untrusted input.
type Limits struct {
	MaxCompressed   int64 // max size of an encoded/compressed payload
	MaxDecompressed int64 // max size of one decompressed XML document
	MaxZipEntries   int   // max number of files inside one zip
}

// DefaultLimits are generous for real DMARC reports (typically < 1 MB).
var DefaultLimits = Limits{
	MaxCompressed:   25 << 20,
	MaxDecompressed: 100 << 20,
	MaxZipEntries:   10,
}

// ErrTooLarge is returned when a payload exceeds the configured limits.
var ErrTooLarge = errors.New("payload exceeds size limit")

// Payload is one candidate DMARC XML document.
type Payload struct {
	Name string // attachment / archive entry name (informational only)
	Data []byte
}

type kind int

const (
	kindUnknown kind = iota
	kindZip
	kindGzip
	kindXML
)

// sniff detects the payload type from magic bytes, falling back to the name.
func sniff(name string, data []byte) kind {
	switch {
	case bytes.HasPrefix(data, []byte("PK\x03\x04")):
		return kindZip
	case len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b:
		return kindGzip
	}
	trimmed := bytes.TrimLeft(data, " \t\r\n\xef\xbb\xbf")
	if bytes.HasPrefix(trimmed, []byte("<?xml")) || bytes.HasPrefix(trimmed, []byte("<feedback")) {
		return kindXML
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return kindZip
	case strings.HasSuffix(lower, ".gz"), strings.HasSuffix(lower, ".gzip"):
		return kindGzip
	case strings.HasSuffix(lower, ".xml"):
		return kindXML
	}
	return kindUnknown
}

// Unpack turns a report file (xml, gzip or zip) into XML payloads.
// It returns (nil, nil) for data that is clearly not a report container.
func Unpack(name string, data []byte, lim Limits) ([]Payload, error) {
	if int64(len(data)) > lim.MaxCompressed {
		return nil, fmt.Errorf("%s: %w (%d bytes compressed)", name, ErrTooLarge, len(data))
	}
	switch sniff(name, data) {
	case kindXML:
		if int64(len(data)) > lim.MaxDecompressed {
			return nil, fmt.Errorf("%s: %w", name, ErrTooLarge)
		}
		return []Payload{{Name: name, Data: data}}, nil
	case kindGzip:
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("%s: gzip: %w", name, err)
		}
		out, err := readLimited(zr, lim.MaxDecompressed)
		inner := strings.TrimSuffix(strings.TrimSuffix(name, ".gz"), ".gzip")
		if zr.Name != "" {
			inner = path.Base(zr.Name)
		}
		closeErr := zr.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: gzip: %w", name, err)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("%s: gzip close: %w", name, closeErr)
		}
		return []Payload{{Name: inner, Data: out}}, nil
	case kindZip:
		return unzip(name, data, lim)
	}
	return nil, nil
}

func unzip(name string, data []byte, lim Limits) ([]Payload, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("%s: zip: %w", name, err)
	}
	var files []*zip.File
	for _, f := range zr.File {
		if !f.FileInfo().IsDir() {
			files = append(files, f)
		}
	}
	if len(files) > lim.MaxZipEntries {
		return nil, fmt.Errorf("%s: zip has %d entries (limit %d)", name, len(files), lim.MaxZipEntries)
	}
	var out []Payload
	for _, f := range files {
		entry := f.Name
		if strings.Contains(entry, "..") || strings.ContainsAny(entry, `/\`) {
			return nil, fmt.Errorf("%s: suspicious zip entry name %q", name, entry)
		}
		if f.UncompressedSize64 > uint64(lim.MaxDecompressed) {
			return nil, fmt.Errorf("%s/%s: %w", name, entry, ErrTooLarge)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", name, entry, err)
		}
		content, err := readLimited(rc, lim.MaxDecompressed)
		_ = rc.Close()
		if err != nil {
			return nil, fmt.Errorf("%s/%s: %w", name, entry, err)
		}
		if sniff(entry, content) == kindXML {
			out = append(out, Payload{Name: entry, Data: content})
		}
	}
	return out, nil
}

// readLimited reads at most limit bytes and fails if more data remains, so a
// lying header (zip) or a gzip bomb cannot exhaust memory.
func readLimited(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrTooLarge
	}
	return data, nil
}
