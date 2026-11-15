package encrypt

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const (
	StreamSegmentSize = 64 << 10
	streamSaltSize    = 32
	streamNonceSize   = 12
	streamKeyInfo     = "stowmark stream v1"
	lastSegmentFlag   = 1
)

var (
	streamMagic             = []byte("STM3")
	ErrStreamClosed         = errors.New("encryption stream is closed")
	ErrTooManyStreamSegment = errors.New("encryption stream has too many segments")
)

func IsStream(data []byte) bool {
	return bytes.HasPrefix(data, streamMagic)
}

type streamCipher struct {
	aead    cipher.AEAD
	aad     []byte
	counter uint64
}

func newStreamCipher(key, salt, aad []byte) (*streamCipher, error) {
	if len(key) != AES256KeySize {
		return nil, fmt.Errorf("%w: got %d bytes, expected %d", ErrInvalidKeySize, len(key), AES256KeySize)
	}
	segmentKey, err := hkdf.Key(sha256.New, key, salt, streamKeyInfo, AES256KeySize)
	if err != nil {
		return nil, fmt.Errorf("derive stream key: %w", err)
	}
	block, err := aes.NewCipher(segmentKey)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create AES-GCM cipher: %w", err)
	}
	return &streamCipher{aead: aead, aad: aad}, nil
}

func (s *streamCipher) nonce(last bool) ([]byte, error) {
	if s.counter == math.MaxUint64 {
		return nil, ErrTooManyStreamSegment
	}
	nonce := make([]byte, streamNonceSize)
	binary.BigEndian.PutUint64(nonce[3:11], s.counter)
	if last {
		nonce[11] = lastSegmentFlag
	}
	s.counter++
	return nonce, nil
}

type StreamWriter struct {
	destination io.Writer
	cipher      *streamCipher
	buffer      []byte
	closed      bool
}

func NewStreamWriter(destination io.Writer, key, aad []byte) (*StreamWriter, error) {
	if destination == nil {
		return nil, ErrDestinationRequired
	}
	salt := make([]byte, streamSaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("generate stream salt: %w", err)
	}
	streamCipher, err := newStreamCipher(key, salt, aad)
	if err != nil {
		return nil, err
	}
	if err := writeFull(destination, append(append([]byte{}, streamMagic...), salt...)); err != nil {
		return nil, fmt.Errorf("write stream header: %w", err)
	}
	return &StreamWriter{
		destination: destination,
		cipher:      streamCipher,
		buffer:      make([]byte, 0, StreamSegmentSize+streamCipher.aead.Overhead()),
	}, nil
}

func (w *StreamWriter) Write(data []byte) (int, error) {
	if w.closed {
		return 0, ErrStreamClosed
	}
	written := 0
	for len(data) > 0 {
		if len(w.buffer) == StreamSegmentSize {
			if err := w.flush(false); err != nil {
				return written, err
			}
		}
		n := copy(w.buffer[len(w.buffer):StreamSegmentSize], data)
		w.buffer = w.buffer[:len(w.buffer)+n]
		data = data[n:]
		written += n
	}
	return written, nil
}

func (w *StreamWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	return w.flush(true)
}

func (w *StreamWriter) flush(last bool) error {
	nonce, err := w.cipher.nonce(last)
	if err != nil {
		return err
	}
	sealed := w.cipher.aead.Seal(w.buffer[:0], nonce, w.buffer, w.cipher.aad)
	if err := writeFull(w.destination, sealed); err != nil {
		return fmt.Errorf("write encrypted segment: %w", err)
	}
	w.buffer = w.buffer[:0]
	return nil
}

type StreamReader struct {
	source  *bufio.Reader
	cipher  *streamCipher
	segment []byte
	pending []byte
	done    bool
}

func NewStreamReader(source io.Reader, key, aad []byte) (*StreamReader, error) {
	header := make([]byte, len(streamMagic)+streamSaltSize)
	if _, err := io.ReadFull(source, header); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: data is shorter than the stream header", ErrDecryptionFailed)
		}
		return nil, fmt.Errorf("read stream header: %w", err)
	}
	if !IsStream(header) {
		return nil, fmt.Errorf("%w: data is not an encrypted stream", ErrDecryptionFailed)
	}
	streamCipher, err := newStreamCipher(key, header[len(streamMagic):], aad)
	if err != nil {
		return nil, err
	}
	return &StreamReader{
		source:  bufio.NewReaderSize(source, StreamSegmentSize+streamCipher.aead.Overhead()+1),
		cipher:  streamCipher,
		segment: make([]byte, StreamSegmentSize+streamCipher.aead.Overhead()),
	}, nil
}

func (r *StreamReader) Read(data []byte) (int, error) {
	for len(r.pending) == 0 {
		if r.done {
			return 0, io.EOF
		}
		if err := r.readSegment(); err != nil {
			return 0, err
		}
	}
	n := copy(data, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *StreamReader) readSegment() error {
	n, err := io.ReadFull(r.source, r.segment)
	last := false
	switch {
	case errors.Is(err, io.EOF):
		return fmt.Errorf("%w: the stream is truncated", ErrDecryptionFailed)
	case errors.Is(err, io.ErrUnexpectedEOF):
		last = true
	case err != nil:
		return fmt.Errorf("read encrypted segment: %w", err)
	default:
		if _, peekErr := r.source.Peek(1); errors.Is(peekErr, io.EOF) {
			last = true
		} else if peekErr != nil {
			return fmt.Errorf("read encrypted segment: %w", peekErr)
		}
	}
	nonce, err := r.cipher.nonce(last)
	if err != nil {
		return err
	}
	plaintext, err := r.cipher.aead.Open(r.segment[:0], nonce, r.segment[:n], r.cipher.aad)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDecryptionFailed, err)
	}
	r.pending = plaintext
	r.done = last
	return nil
}
