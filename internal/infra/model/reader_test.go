package model_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/bruli-lab/stowmark/internal/infra/model"
	"github.com/stretchr/testify/require"
)

var errCloseFailed = errors.New("close failed")

type recordingCloser struct {
	closed bool
}

func (r *recordingCloser) Close() error {
	r.closed = true
	return errCloseFailed
}

func TestContextReader_Read(t *testing.T) {
	t.Parallel()

	t.Run(`Given a ContextReader with an active context,
	when it is read,
	then it returns the underlying data`, func(t *testing.T) {
		t.Parallel()

		got, err := io.ReadAll(model.ContextReader{Ctx: context.Background(), Reader: bytes.NewReader([]byte("data"))})

		require.NoError(t, err)
		require.Equal(t, "data", string(got))
	})

	t.Run(`Given a ContextReader with a canceled context,
	when it is read,
	then it returns a context canceled error`, func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := io.ReadAll(model.ContextReader{Ctx: ctx, Reader: bytes.NewReader([]byte("data"))})

		require.ErrorIs(t, err, context.Canceled)
	})
}

func TestReadCloser_Close(t *testing.T) {
	t.Parallel()

	t.Run(`Given a ReadCloser,
	when Close method is called,
	then it closes the underlying closer and returns its error`, func(t *testing.T) {
		t.Parallel()

		closer := &recordingCloser{}
		reader := &model.ReadCloser{Reader: bytes.NewReader(nil), Closer: closer}

		err := reader.Close()

		require.ErrorIs(t, err, errCloseFailed)
		require.True(t, closer.closed)
	})
}
