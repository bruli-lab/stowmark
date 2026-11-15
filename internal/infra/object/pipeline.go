package object

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"

	"github.com/bruli-lab/stowmark/internal/domain/repository"
	"github.com/bruli-lab/stowmark/internal/infra/compression"
	"github.com/bruli-lab/stowmark/internal/infra/encrypt"
	"github.com/bruli-lab/stowmark/internal/infra/model"
)

var (
	ErrHashMismatch        = errors.New("object hash mismatch")
	ErrCompressionRequired = errors.New("compression configuration is required")
)

type Pipeline struct {
	handlersFactory   *compression.HandlersFactory
	encryptionHandler *encrypt.AESGCMHandler
}

func (p Pipeline) Write(ctx context.Context, destination io.Writer, source io.Reader, codec repository.Codec, expectedHash string) error {
	comp := codec.Compression()
	if comp == nil {
		return ErrCompressionRequired
	}
	handler, err := p.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return err
	}
	if codec.ContentAddressed() {
		return p.writeContent(ctx, destination, source, codec, handler, expectedHash)
	}
	return p.writeCompressed(ctx, destination, source, codec, handler, expectedHash)
}

func (p Pipeline) writeCompressed(
	ctx context.Context,
	destination io.Writer,
	source io.Reader,
	codec repository.Codec,
	handler compression.Handler,
	expectedHash string,
) error {
	output := destination
	var encryption *encrypt.Encoder
	if codec.EncryptsData() {
		var err error
		encryption, err = p.encryptionHandler.Encode(destination, codec.DataKey())
		if err != nil {
			return fmt.Errorf("create encryption writer: %w", err)
		}
		output = encryption.Writer
	}

	hasher := sha256.New()
	if err := compress(ctx, io.MultiWriter(output, hasher), source, codec.Compression(), handler); err != nil {
		return err
	}
	if err := checkHash(hasher, expectedHash); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if encryption == nil {
		return nil
	}
	if err := encryption.Closer(); err != nil {
		return fmt.Errorf("finish encryption: %w", err)
	}
	return nil
}

func (p Pipeline) writeContent(
	ctx context.Context,
	destination io.Writer,
	source io.Reader,
	codec repository.Codec,
	handler compression.Handler,
	expectedHash string,
) error {
	hasher, err := NewContentHasher(codec)
	if err != nil {
		return err
	}

	output := destination
	var stream *encrypt.StreamWriter
	if codec.EncryptsData() {
		stream, err = encrypt.NewStreamWriter(destination, codec.DataKey(), []byte(expectedHash))
		if err != nil {
			return fmt.Errorf("create encryption stream: %w", err)
		}
		output = stream
	}

	if err := writeCompressionHeader(output, codec.Compression().CompType()); err != nil {
		return err
	}
	if err := compress(ctx, output, io.TeeReader(source, hasher), codec.Compression(), handler); err != nil {
		return err
	}
	if err := checkHash(hasher, expectedHash); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if stream == nil {
		return nil
	}
	if err := stream.Close(); err != nil {
		return fmt.Errorf("finish encryption: %w", err)
	}
	return nil
}

func compress(ctx context.Context, destination io.Writer, source io.Reader, comp *repository.Compression, handler compression.Handler) error {
	compressor, err := handler.Encode(destination, comp.Level())
	if err != nil {
		return fmt.Errorf("create %q compression writer: %w", comp.CompType(), err)
	}
	if _, err := io.Copy(compressor.Writer, model.ContextReader{Ctx: ctx, Reader: source}); err != nil {
		_ = compressor.Closer()
		return fmt.Errorf("compress object: %w", err)
	}
	if err := compressor.Closer(); err != nil {
		return fmt.Errorf("finish %q compression: %w", comp.CompType(), err)
	}
	return nil
}

func (p Pipeline) Read(ctx context.Context, destination io.Writer, source io.Reader, codec repository.Codec, expectedHash string) (int64, error) {
	if codec.ContentAddressed() {
		return p.readContent(ctx, destination, source, codec, expectedHash)
	}
	return p.readCompressed(ctx, destination, source, codec, expectedHash)
}

func (p Pipeline) readCompressed(ctx context.Context, destination io.Writer, source io.Reader, codec repository.Codec, expectedHash string) (int64, error) {
	comp := codec.Compression()
	if comp == nil {
		return 0, ErrCompressionRequired
	}
	handler, err := p.handlersFactory.GetHandler(comp.CompType())
	if err != nil {
		return 0, err
	}

	input, err := p.decryptCompressed(source, codec)
	if err != nil {
		return 0, err
	}

	hasher := sha256.New()
	compressed := io.TeeReader(model.ContextReader{Ctx: ctx, Reader: input}, hasher)
	written, err := decompress(destination, compressed, comp.CompType(), handler)
	if err != nil {
		return written, err
	}
	if _, err := io.Copy(io.Discard, compressed); err != nil {
		return written, fmt.Errorf("read remaining object data: %w", err)
	}
	return written, checkHash(hasher, expectedHash)
}

func (p Pipeline) readContent(ctx context.Context, destination io.Writer, source io.Reader, codec repository.Codec, expectedHash string) (int64, error) {
	hasher, err := NewContentHasher(codec)
	if err != nil {
		return 0, err
	}

	input, err := DecryptContent(source, codec, expectedHash)
	if err != nil {
		return 0, err
	}
	input = model.ContextReader{Ctx: ctx, Reader: input}

	compType, err := readCompressionHeader(input)
	if err != nil {
		return 0, err
	}
	handler, err := p.handlersFactory.GetHandler(compType)
	if err != nil {
		return 0, err
	}

	written, err := decompress(io.MultiWriter(destination, hasher), input, compType, handler)
	if err != nil {
		return written, err
	}
	if _, err := io.Copy(io.Discard, input); err != nil {
		return written, fmt.Errorf("read remaining object data: %w", err)
	}
	return written, checkHash(hasher, expectedHash)
}

func (p Pipeline) Verify(ctx context.Context, source io.Reader, codec repository.Codec, expectedHash string) error {
	if codec.ContentAddressed() {
		_, err := p.readContent(ctx, io.Discard, source, codec, expectedHash)
		return err
	}
	input, err := p.decryptCompressed(source, codec)
	if err != nil {
		return err
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, model.ContextReader{Ctx: ctx, Reader: input}); err != nil {
		return fmt.Errorf("read object data: %w", err)
	}
	return checkHash(hasher, expectedHash)
}

func (p Pipeline) decryptCompressed(source io.Reader, codec repository.Codec) (io.Reader, error) {
	if !codec.EncryptsData() {
		return source, nil
	}
	decrypted, err := p.encryptionHandler.Decode(source, codec.DataKey())
	if err != nil {
		return nil, fmt.Errorf("decrypt object: %w", err)
	}
	return decrypted, nil
}

func DecryptContent(source io.Reader, codec repository.Codec, objectHash string) (io.Reader, error) {
	if !codec.EncryptsData() {
		return source, nil
	}
	decrypted, err := encrypt.NewStreamReader(source, codec.DataKey(), []byte(objectHash))
	if err != nil {
		return nil, fmt.Errorf("decrypt object: %w", err)
	}
	return decrypted, nil
}

func EncryptContent(destination io.Writer, codec repository.Codec, objectHash string) (io.WriteCloser, error) {
	stream, err := encrypt.NewStreamWriter(destination, codec.DataKey(), []byte(objectHash))
	if err != nil {
		return nil, fmt.Errorf("create encryption stream: %w", err)
	}
	return stream, nil
}

func decompress(destination io.Writer, source io.Reader, compType repository.CompressionType, handler compression.Handler) (int64, error) {
	decoded, err := handler.Decode(source)
	if err != nil {
		return 0, fmt.Errorf("create %q decompression reader: %w", compType, err)
	}
	defer decoded.Closer()

	written, err := io.Copy(destination, decoded.Reader)
	if err != nil {
		return written, fmt.Errorf("decompress object: %w", err)
	}
	return written, nil
}

func checkHash(hasher hash.Hash, expectedHash string) error {
	calculatedHash := hex.EncodeToString(hasher.Sum(nil))
	if calculatedHash != expectedHash {
		return fmt.Errorf("%w: expected %s, calculated %s", ErrHashMismatch, expectedHash, calculatedHash)
	}
	return nil
}

func NewPipeline(handlersFactory *compression.HandlersFactory) *Pipeline {
	return &Pipeline{
		handlersFactory:   handlersFactory,
		encryptionHandler: encrypt.NewAESGCMHandler(),
	}
}
