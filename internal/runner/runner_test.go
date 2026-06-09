package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const testPort = 12345

var _ SingBoxRunner = (*processRunner)(nil)

func TestProcessRunner_Start(t *testing.T) {
	t.Run("command start error", func(t *testing.T) {
		// arrange
		startErr := errors.New("command not found")
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(startErr)

		r := NewProcessRunner("sing-box", WithCommandFactory(func(_ string, _ []string) ProcessController {
			return cmdMock
		}))

		// act
		err := r.Start(t.Context(), "/tmp/config.json", testPort)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, startErr)
		assert.Contains(t, err.Error(), "start sing-box")
	})

	t.Run("already started", func(t *testing.T) {
		// arrange
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)

		mockConn := NewMockConn(t)
		mockConn.
			EXPECT().
			Close().
			Return(nil)

		successfulDialer := NewMockDialer(t)
		successfulDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(mockConn, nil)

		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(successfulDialer),
		)

		require.NoError(t, r.Start(t.Context(), "/tmp/config.json", testPort))

		// act
		err := r.Start(t.Context(), "/tmp/config.json", testPort)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrAlreadyStarted)
	})

	t.Run("wait for ready timeout", func(t *testing.T) {
		// arrange
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)
		cmdMock.
			EXPECT().
			Kill().
			Return(nil)
		cmdMock.
			EXPECT().
			Wait().
			Return(nil)
		cmdMock.
			EXPECT().
			Stderr().
			Return("some error output")

		failingDialer := NewMockDialer(t)
		failingDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(nil, errors.New("connection refused"))

		now := time.Now()
		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(failingDialer),
			WithNowFunc(func() time.Time { return now }),
			WithPollDelay(1*time.Millisecond),
		)

		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()

		// act
		err := r.Start(ctx, "/tmp/config.json", testPort)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrStartTimeout)
		assert.Contains(t, err.Error(), "some error output")
	})

	t.Run("success", func(t *testing.T) {
		// arrange
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)

		mockConn := NewMockConn(t)
		mockConn.
			EXPECT().
			Close().
			Return(nil)

		successfulDialer := NewMockDialer(t)
		successfulDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(mockConn, nil)

		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(successfulDialer),
			WithPollDelay(1*time.Millisecond),
		)

		// act
		err := r.Start(t.Context(), "/tmp/config.json", testPort)

		// assert
		require.NoError(t, err)
	})

	t.Run("default cli path", func(t *testing.T) {
		// arrange
		var receivedName string
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)

		mockConn := NewMockConn(t)
		mockConn.
			EXPECT().
			Close().
			Return(nil)

		successfulDialer := NewMockDialer(t)
		successfulDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(mockConn, nil)

		r := NewProcessRunner("",
			WithCommandFactory(func(name string, _ []string) ProcessController {
				receivedName = name
				return cmdMock
			}),
			WithDialer(successfulDialer),
			WithPollDelay(1*time.Millisecond),
		)

		// act
		err := r.Start(t.Context(), "/tmp/config.json", testPort)

		// assert
		require.NoError(t, err)
		assert.Equal(t, defaultCLIPath, receivedName)
	})

	t.Run("context canceled", func(t *testing.T) {
		// arrange
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)
		cmdMock.
			EXPECT().
			Kill().
			Return(nil)
		cmdMock.
			EXPECT().
			Wait().
			Return(nil)
		cmdMock.
			EXPECT().
			Stderr().
			Return("")

		failingDialer := NewMockDialer(t)
		failingDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(nil, errors.New("connection refused"))

		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(failingDialer),
			WithPollDelay(1*time.Millisecond),
		)

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		// act
		err := r.Start(ctx, "/tmp/config.json", testPort)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrStartTimeout)
	})

	t.Run("stderr captured on timeout", func(t *testing.T) {
		// arrange
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)
		cmdMock.
			EXPECT().
			Kill().
			Return(nil)
		cmdMock.
			EXPECT().
			Wait().
			Return(nil)
		cmdMock.
			EXPECT().
			Stderr().
			Return("sing-box: invalid config")

		failingDialer := NewMockDialer(t)
		failingDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(nil, errors.New("connection refused"))

		now := time.Now()
		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(failingDialer),
			WithNowFunc(func() time.Time { return now }),
			WithPollDelay(1*time.Millisecond),
		)

		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()

		// act
		err := r.Start(ctx, "/tmp/config.json", testPort)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrStartTimeout)
		assert.Contains(t, err.Error(), "sing-box: invalid config")
	})

	t.Run("empty stderr on timeout", func(t *testing.T) {
		// arrange
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)
		cmdMock.
			EXPECT().
			Kill().
			Return(nil)
		cmdMock.
			EXPECT().
			Wait().
			Return(nil)
		cmdMock.
			EXPECT().
			Stderr().
			Return("")

		failingDialer := NewMockDialer(t)
		failingDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(nil, errors.New("connection refused"))

		now := time.Now()
		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(failingDialer),
			WithNowFunc(func() time.Time { return now }),
			WithPollDelay(1*time.Millisecond),
		)

		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()

		// act
		err := r.Start(ctx, "/tmp/config.json", testPort)

		// assert
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrStartTimeout)
	})
}

func TestProcessRunner_Stop(t *testing.T) {
	t.Run("not started", func(t *testing.T) {
		// arrange
		r := NewProcessRunner("sing-box")

		// act
		err := r.Stop(t.Context())

		// assert
		require.NoError(t, err)
	})

	t.Run("graceful stop", func(t *testing.T) {
		// arrange
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)
		cmdMock.
			EXPECT().
			Signal(mock.Anything).
			Return(nil)
		cmdMock.
			EXPECT().
			Wait().
			Return(nil)

		mockConn := NewMockConn(t)
		mockConn.
			EXPECT().
			Close().
			Return(nil)

		successfulDialer := NewMockDialer(t)
		successfulDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(mockConn, nil)

		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(successfulDialer),
			WithPollDelay(1*time.Millisecond),
		)

		require.NoError(t, r.Start(t.Context(), "/tmp/config.json", testPort))

		// act
		err := r.Stop(t.Context())

		// assert
		require.NoError(t, err)
	})

	t.Run("kill after grace period", func(t *testing.T) {
		// arrange
		waitBlocker := make(chan struct{})

		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)
		cmdMock.
			EXPECT().
			Signal(mock.Anything).
			Return(nil).Run(func(_ os.Signal) {
			// process ignores SIGINT, Wait не завершается сам.
		})
		cmdMock.
			EXPECT().
			Kill().
			Return(nil).Run(func() {
			close(waitBlocker)
		})
		cmdMock.
			EXPECT().
			Wait().
			Return(nil).Run(func() {
			<-waitBlocker
		})

		mockConn := NewMockConn(t)
		mockConn.
			EXPECT().
			Close().
			Return(nil)

		successfulDialer := NewMockDialer(t)
		successfulDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(mockConn, nil)

		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(successfulDialer),
			WithPollDelay(1*time.Millisecond),
			WithStartTimeout(50*time.Millisecond),
			WithStopGracePeriod(10*time.Millisecond),
		)

		require.NoError(t, r.Start(t.Context(), "/tmp/config.json", testPort))

		// act
		err := r.Stop(t.Context())

		// assert
		require.NoError(t, err)
	})
}

func TestProcessRunner_Options(t *testing.T) {
	t.Run("poll delay", func(t *testing.T) {
		// arrange
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)

		mockConn := NewMockConn(t)
		mockConn.
			EXPECT().
			Close().
			Return(nil)

		successfulDialer := NewMockDialer(t)
		successfulDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(mockConn, nil)

		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(successfulDialer),
			WithPollDelay(10*time.Millisecond),
		)

		// act
		err := r.Start(t.Context(), "/tmp/config.json", testPort)

		// assert
		require.NoError(t, err)
	})

	t.Run("now func", func(t *testing.T) {
		// arrange
		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)

		mockConn := NewMockConn(t)
		mockConn.
			EXPECT().
			Close().
			Return(nil)

		successfulDialer := NewMockDialer(t)
		successfulDialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", testPort)).
			Return(mockConn, nil)

		now := time.Now()
		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(successfulDialer),
			WithNowFunc(func() time.Time { return now }),
			WithPollDelay(1*time.Millisecond),
		)

		// act
		err := r.Start(t.Context(), "/tmp/config.json", testPort)

		// assert
		require.NoError(t, err)
	})

	t.Run("custom socks port", func(t *testing.T) {
		// arrange
		customPort := 19999

		cmdMock := NewMockProcessController(t)
		cmdMock.
			EXPECT().
			Start(mock.Anything).
			Return(nil)

		conn := NewMockConn(t)
		conn.
			EXPECT().
			Close().
			Return(nil)

		dialer := NewMockDialer(t)
		dialer.
			EXPECT().
			DialContext(mock.Anything, mock.Anything, fmt.Sprintf("127.0.0.1:%d", customPort)).
			Return(conn, nil)

		r := NewProcessRunner("sing-box",
			WithCommandFactory(func(_ string, _ []string) ProcessController { return cmdMock }),
			WithDialer(dialer),
			WithPollDelay(1*time.Millisecond),
		)

		// act
		err := r.Start(t.Context(), "/tmp/config.json", customPort)

		// assert
		require.NoError(t, err)
	})
}
