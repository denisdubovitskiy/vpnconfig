package resolver

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestOptionsFromURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		urls      []string
		wantCount int
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "single DoH URL",
			urls:      []string{"https://dns.google/dns-query"},
			wantCount: 1,
		},
		{
			name:      "single DoT URL",
			urls:      []string{"tls://dns.google"},
			wantCount: 1,
		},
		{
			name:      "single plain DNS IP",
			urls:      []string{"1.1.1.1"},
			wantCount: 1,
		},
		{
			name:      "plain DNS with port",
			urls:      []string{"1.1.1.1:53"},
			wantCount: 1,
		},
		{
			name:      "plain DNS hostname",
			urls:      []string{"dns.google:53"},
			wantCount: 1,
		},
		{
			name: "mixed DoH, DoT and plain",
			urls: []string{
				"https://dns.cloudflare.com/dns-query",
				"tls://dns.google",
				"1.1.1.1",
			},
			wantCount: 3,
		},
		{
			name:      "DoH without path is invalid",
			urls:      []string{"https://dns.google"},
			wantErr:   true,
			errSubstr: "path is required",
		},
		{
			name:      "unsupported scheme",
			urls:      []string{"ftp://dns.google/dns-query"},
			wantErr:   true,
			errSubstr: "unsupported scheme",
		},
		{
			name:      "empty url in list",
			urls:      []string{"1.1.1.1", ""},
			wantErr:   true,
			errSubstr: "empty url",
		},
		{
			name:      "whitespace-only url",
			urls:      []string{"   "},
			wantErr:   true,
			errSubstr: "empty url",
		},
		{
			name:      "empty list",
			urls:      []string{},
			wantCount: 0,
		},
		{
			name:      "nil list",
			urls:      nil,
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			opts, err := OptionsFromURLs(tc.urls)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errSubstr != "" {
					assert.True(t, strings.Contains(err.Error(), tc.errSubstr),
						"expected error to contain %q, got %q", tc.errSubstr, err.Error())
				}
				return
			}
			require.NoError(t, err)
			require.Len(t, opts, tc.wantCount)
		})
	}
}

func TestNew_EmptyChain(t *testing.T) {
	t.Parallel()

	r := New()

	require.NotNil(t, r)
	_, err := r.LookupIP(t.Context(), "ip", "example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no dns resolvers configured")
}

func TestNew_BuildsChainInOrder(t *testing.T) {
	t.Parallel()

	first := NewMockIPResolver(t)
	second := NewMockIPResolver(t)
	third := NewMockIPResolver(t)

	first.EXPECT().LookupIP(mock.Anything, "ip", "example.com").
		Return(nil, errors.New("first fail"))
	second.EXPECT().LookupIP(mock.Anything, "ip", "example.com").
		Return(nil, errors.New("second fail"))
	third.EXPECT().LookupIP(mock.Anything, "ip", "example.com").
		Return([]net.IP{net.ParseIP("1.1.1.1").To4()}, nil)

	r := New(
		WithResolver(first),
		WithResolver(second),
		WithResolver(third),
	)
	ips, err := r.LookupIP(t.Context(), "ip", "example.com")

	require.NoError(t, err)
	assert.Equal(t, []net.IP{net.ParseIP("1.1.1.1").To4()}, ips)
}

func TestLookupIP(t *testing.T) {
	t.Parallel()

	someIP := net.ParseIP("1.1.1.1").To4()
	otherIP := net.ParseIP("8.8.8.8").To4()
	host := "example.com"

	t.Run("returns first successful result", func(t *testing.T) {
		t.Parallel()

		want := []net.IP{someIP}
		m := NewMockIPResolver(t)
		m.EXPECT().LookupIP(mock.Anything, "ip", host).Return(want, nil)

		r := New(WithResolver(m))

		got, err := r.LookupIP(t.Context(), "ip", host)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("falls back when primary returns error", func(t *testing.T) {
		t.Parallel()

		primary := NewMockIPResolver(t)
		fallback := NewMockIPResolver(t)
		primary.EXPECT().LookupIP(mock.Anything, "ip", host).
			Return(nil, errors.New("primary failed"))
		fallback.EXPECT().LookupIP(mock.Anything, "ip", host).
			Return([]net.IP{someIP}, nil)

		r := New(
			WithResolver(primary),
			WithResolver(fallback),
		)

		got, err := r.LookupIP(t.Context(), "ip", host)

		require.NoError(t, err)
		assert.Equal(t, []net.IP{someIP}, got)
	})

	t.Run("falls back when primary returns empty slice", func(t *testing.T) {
		t.Parallel()

		primary := NewMockIPResolver(t)
		fallback := NewMockIPResolver(t)
		primary.EXPECT().LookupIP(mock.Anything, "ip", host).Return(nil, nil)
		fallback.EXPECT().LookupIP(mock.Anything, "ip", host).Return([]net.IP{otherIP}, nil)

		r := New(
			WithResolver(primary),
			WithResolver(fallback),
		)

		got, err := r.LookupIP(t.Context(), "ip", host)

		require.NoError(t, err)
		assert.Equal(t, []net.IP{otherIP}, got)
	})

	t.Run("returns last error when all resolvers fail", func(t *testing.T) {
		t.Parallel()

		firstErr := errors.New("first failed")
		middleErr := errors.New("middle failed")
		lastErr := errors.New("last failed")
		first := NewMockIPResolver(t)
		middle := NewMockIPResolver(t)
		last := NewMockIPResolver(t)
		first.EXPECT().LookupIP(mock.Anything, "ip", host).Return(nil, firstErr)
		middle.EXPECT().LookupIP(mock.Anything, "ip", host).Return(nil, middleErr)
		last.EXPECT().LookupIP(mock.Anything, "ip", host).Return(nil, lastErr)

		r := New(
			WithResolver(first),
			WithResolver(middle),
			WithResolver(last),
		)

		got, err := r.LookupIP(t.Context(), "ip", host)

		require.Error(t, err)
		assert.Nil(t, got)
		assert.ErrorIs(t, err, lastErr)
	})

	t.Run("propagates context to resolver", func(t *testing.T) {
		t.Parallel()

		type ctxKey struct{}
		wantCtx := context.WithValue(t.Context(), ctxKey{}, "marker")
		var gotFromCtx any
		m := NewMockIPResolver(t)
		m.EXPECT().LookupIP(mock.Anything, "ip", host).
			Run(func(ctx context.Context, _, _ string) {
				gotFromCtx = ctx.Value(ctxKey{})
			}).
			Return([]net.IP{someIP}, nil)

		r := New(WithResolver(m))

		_, err := r.LookupIP(wantCtx, "ip", host)

		require.NoError(t, err)
		assert.Equal(t, "marker", gotFromCtx)
	})

	t.Run("passes network and host to resolver", func(t *testing.T) {
		t.Parallel()

		var gotNetwork, gotHost string
		m := NewMockIPResolver(t)
		m.EXPECT().LookupIP(mock.Anything, "ip4", "vpn.example.com").
			Run(func(_ context.Context, network, host string) {
				gotNetwork = network
				gotHost = host
			}).
			Return([]net.IP{someIP}, nil)

		r := New(WithResolver(m))

		_, err := r.LookupIP(t.Context(), "ip4", "vpn.example.com")

		require.NoError(t, err)
		assert.Equal(t, "ip4", gotNetwork)
		assert.Equal(t, "vpn.example.com", gotHost)
	})
}

func TestParseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		raw       string
		wantKind  string
		wantHost  string
		wantPath  string
		wantErr   bool
		errSubstr string
	}{
		{
			name:     "DoH full url",
			raw:      "https://dns.google/dns-query",
			wantKind: "doh",
			wantHost: "dns.google",
			wantPath: "/dns-query",
		},
		{
			name:     "DoH with port",
			raw:      "https://dns.google:443/dns-query",
			wantKind: "doh",
			wantHost: "dns.google:443",
			wantPath: "/dns-query",
		},
		{
			name:     "DoT without port",
			raw:      "tls://dns.google",
			wantKind: "dot",
			wantHost: "dns.google",
		},
		{
			name:     "DoT with port",
			raw:      "tls://dns.google:853",
			wantKind: "dot",
			wantHost: "dns.google:853",
		},
		{
			name:     "plain IP",
			raw:      "1.1.1.1",
			wantKind: "plain",
			wantHost: "1.1.1.1:53",
		},
		{
			name:     "plain IP with port",
			raw:      "1.1.1.1:5353",
			wantKind: "plain",
			wantHost: "1.1.1.1:5353",
		},
		{
			name:     "plain hostname with port",
			raw:      "dns.google:53",
			wantKind: "plain",
			wantHost: "dns.google:53",
		},
		{
			name:     "plain hostname without port",
			raw:      "dns.google",
			wantKind: "plain",
			wantHost: "dns.google:53",
		},
		{
			name:      "DoH without path",
			raw:       "https://dns.google",
			wantErr:   true,
			errSubstr: "path is required",
		},
		{
			name:      "DoH with empty path",
			raw:       "https://dns.google/",
			wantErr:   true,
			errSubstr: "path is required",
		},
		{
			name:      "unsupported scheme",
			raw:       "ftp://dns.google/dns-query",
			wantErr:   true,
			errSubstr: "unsupported scheme",
		},
		{
			name:      "http scheme is not DoH",
			raw:       "http://dns.google/dns-query",
			wantErr:   true,
			errSubstr: "unsupported scheme",
		},
		{
			name:      "empty url",
			raw:       "",
			wantErr:   true,
			errSubstr: "empty url",
		},
		{
			name:      "whitespace only",
			raw:       "   ",
			wantErr:   true,
			errSubstr: "empty url",
		},
		{
			name:      "plain url with scheme prefix",
			raw:       "udp://1.1.1.1",
			wantErr:   true,
			errSubstr: "unsupported scheme",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := parseURL(tc.raw)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errSubstr != "" {
					assert.True(t, strings.Contains(err.Error(), tc.errSubstr),
						"expected error to contain %q, got %q", tc.errSubstr, err.Error())
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, parsed)
			assert.Equal(t, tc.wantKind, parsed.kind)
			assert.Equal(t, tc.wantHost, parsed.host)
			if tc.wantPath != "" {
				assert.Equal(t, tc.wantPath, parsed.path)
			}
		})
	}
}
