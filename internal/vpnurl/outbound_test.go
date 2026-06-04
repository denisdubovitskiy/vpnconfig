package vpnurl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVLESSOutbound_Tag(t *testing.T) {
	t.Parallel()

	// Проверяем получение тега.
	t.Run("returns tag", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &VLESSOutbound{OutboundTag: "my-tag"}

		// act & assert
		assert.Equal(t, "my-tag", o.Tag())
	})

	// Проверяем пустой тег.
	t.Run("empty tag", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &VLESSOutbound{}

		// act & assert
		assert.Empty(t, o.Tag())
	})
}

func TestVLESSOutbound_Type(t *testing.T) {
	t.Parallel()

	// Проверяем получение типа.
	t.Run("returns type", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &VLESSOutbound{OutboundType: "vless"}

		// act & assert
		assert.Equal(t, "vless", o.Type())
	})

	// Проверяем пустой тип.
	t.Run("empty type", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &VLESSOutbound{}

		// act & assert
		assert.Empty(t, o.Type())
	})
}

func TestVLESSOutbound_ToOutbound(t *testing.T) {
	t.Parallel()

	// Проверяем, что ToOutbound возвращает получатель с сохранением данных.
	t.Run("returns receiver with data", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &VLESSOutbound{
			OutboundType: "vless",
			OutboundTag:  "test-out",
			Server:       "1.1.1.1",
		}

		// act
		got := o.ToOutbound()

		// assert
		require.NotNil(t, got)
		vless, ok := got.(*VLESSOutbound)
		require.True(t, ok)
		assert.Same(t, o, vless)
		assert.Equal(t, "vless", vless.Type())
		assert.Equal(t, "test-out", vless.Tag())
	})
}

func TestTrojanOutbound_Tag(t *testing.T) {
	t.Parallel()

	// Проверяем получение тега.
	t.Run("returns tag", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &TrojanOutbound{OutboundTag: "my-tag"}

		// act & assert
		assert.Equal(t, "my-tag", o.Tag())
	})

	// Проверяем пустой тег.
	t.Run("empty tag", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &TrojanOutbound{}

		// act & assert
		assert.Empty(t, o.Tag())
	})
}

func TestTrojanOutbound_Type(t *testing.T) {
	t.Parallel()

	// Проверяем получение типа.
	t.Run("returns type", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &TrojanOutbound{OutboundType: "trojan"}

		// act & assert
		assert.Equal(t, "trojan", o.Type())
	})

	// Проверяем пустой тип.
	t.Run("empty type", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &TrojanOutbound{}

		// act & assert
		assert.Empty(t, o.Type())
	})
}

func TestTrojanOutbound_ToOutbound(t *testing.T) {
	t.Parallel()

	// Проверяем, что ToOutbound возвращает получатель с сохранением данных.
	t.Run("returns receiver with data", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &TrojanOutbound{
			OutboundType: "trojan",
			OutboundTag:  "test-out",
			Server:       "1.1.1.1",
		}

		// act
		got := o.ToOutbound()

		// assert
		require.NotNil(t, got)
		trojan, ok := got.(*TrojanOutbound)
		require.True(t, ok)
		assert.Same(t, o, trojan)
		assert.Equal(t, "trojan", trojan.Type())
		assert.Equal(t, "test-out", trojan.Tag())
	})
}

func TestShadowsocksOutbound_Tag(t *testing.T) {
	t.Parallel()

	// Проверяем получение тега.
	t.Run("returns tag", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &ShadowsocksOutbound{OutboundTag: "my-tag"}

		// act & assert
		assert.Equal(t, "my-tag", o.Tag())
	})

	// Проверяем пустой тег.
	t.Run("empty tag", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &ShadowsocksOutbound{}

		// act & assert
		assert.Empty(t, o.Tag())
	})
}

func TestShadowsocksOutbound_Type(t *testing.T) {
	t.Parallel()

	// Проверяем получение типа.
	t.Run("returns type", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &ShadowsocksOutbound{OutboundType: "shadowsocks"}

		// act & assert
		assert.Equal(t, "shadowsocks", o.Type())
	})

	// Проверяем пустой тип.
	t.Run("empty type", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &ShadowsocksOutbound{}

		// act & assert
		assert.Empty(t, o.Type())
	})
}

func TestShadowsocksOutbound_ToOutbound(t *testing.T) {
	t.Parallel()

	// Проверяем, что ToOutbound возвращает получатель с сохранением данных.
	t.Run("returns receiver with data", func(t *testing.T) {
		t.Parallel()

		// arrange
		o := &ShadowsocksOutbound{
			OutboundType: "shadowsocks",
			OutboundTag:  "test-out",
			Server:       "1.1.1.1",
		}

		// act
		got := o.ToOutbound()

		// assert
		require.NotNil(t, got)
		ss, ok := got.(*ShadowsocksOutbound)
		require.True(t, ok)
		assert.Same(t, o, ss)
		assert.Equal(t, "shadowsocks", ss.Type())
		assert.Equal(t, "test-out", ss.Tag())
	})
}
