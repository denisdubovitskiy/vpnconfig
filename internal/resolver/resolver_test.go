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

	first.
		EXPECT().
		LookupIP(mock.Anything, "ip", "example.com").
		Return(nil, errors.New("first fail"))
	second.
		EXPECT().
		LookupIP(mock.Anything, "ip", "example.com").
		Return(nil, errors.New("second fail"))
	third.
		EXPECT().
		LookupIP(mock.Anything, "ip", "example.com").
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
		m.
			EXPECT().
			LookupIP(mock.Anything, "ip", host).
			Return(want, nil)

		r := New(WithResolver(m))

		got, err := r.LookupIP(t.Context(), "ip", host)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("falls back when primary returns error", func(t *testing.T) {
		t.Parallel()

		primary := NewMockIPResolver(t)
		fallback := NewMockIPResolver(t)
		primary.
			EXPECT().
			LookupIP(mock.Anything, "ip", host).
			Return(nil, errors.New("primary failed"))
		fallback.
			EXPECT().
			LookupIP(mock.Anything, "ip", host).
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
		primary.
			EXPECT().
			LookupIP(mock.Anything, "ip", host).Return(nil, nil)
		fallback.
			EXPECT().
			LookupIP(mock.Anything, "ip", host).Return([]net.IP{otherIP}, nil)

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
		first.
			EXPECT().
			LookupIP(mock.Anything, "ip", host).Return(nil, firstErr)
		middle.
			EXPECT().
			LookupIP(mock.Anything, "ip", host).Return(nil, middleErr)
		last.
			EXPECT().
			LookupIP(mock.Anything, "ip", host).Return(nil, lastErr)

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
		m.
			EXPECT().
			LookupIP(mock.Anything, "ip", host).
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
		m.
			EXPECT().
			LookupIP(mock.Anything, "ip4", "vpn.example.com").
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

func TestEnsurePort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		host        string
		defaultPort string
		want        string
	}{
		{name: "empty host", host: "", defaultPort: "53", want: ""},
		{name: "plain ip adds port", host: "1.1.1.1", defaultPort: "53", want: "1.1.1.1:53"},
		{name: "plain ip keeps port", host: "1.1.1.1:5353", defaultPort: "53", want: "1.1.1.1:5353"},
		{name: "hostname adds port", host: "dns.google", defaultPort: "53", want: "dns.google:53"},
		{name: "hostname keeps port", host: "dns.google:853", defaultPort: "53", want: "dns.google:853"},
		{name: "ipv6 bracket without port", host: "[::1]", defaultPort: "53", want: "[::1]:53"},
		{name: "ipv6 bracket with port", host: "[::1]:5353", defaultPort: "53", want: "[::1]:5353"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := ensurePort(tc.host, tc.defaultPort)

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuildDNSQuery(t *testing.T) {
	t.Parallel()

	// arrange
	const host = "example.com"

	// act
	query := buildDNSQuery(host)

	// assert
	require.GreaterOrEqual(t, len(query), 12+dnsQuestionTail, "query must contain header and one question")
	assert.Equal(t, byte(0x01), query[5], "expected one question")

	// Проверяем, что QNAME содержит метки example и com.
	require.True(t, len(query) > 12)
	offset := 12
	labelLen := int(query[offset])
	offset++
	assert.Equal(t, "example", string(query[offset:offset+labelLen]))
	offset += labelLen
	labelLen = int(query[offset])
	offset++
	assert.Equal(t, "com", string(query[offset:offset+labelLen]))
}

func TestSkipDNSName(t *testing.T) {
	t.Parallel()

	t.Run("simple name", func(t *testing.T) {
		t.Parallel()

		// example.com encoded as [7]example[3]com[0]
		buf := []byte{7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0, 'x'}
		got, err := skipDNSName(buf, 0)

		require.NoError(t, err)
		assert.Equal(t, 13, got)
	})

	t.Run("pointer compression", func(t *testing.T) {
		t.Parallel()

		// offset 4 указывает на pointer 0xC000 (смещение 0).
		buf := []byte{3, 'f', 'o', 'o', 0xC0, 0x00}
		got, err := skipDNSName(buf, 4)

		require.NoError(t, err)
		assert.Equal(t, 6, got)
	})

	t.Run("out of bounds", func(t *testing.T) {
		t.Parallel()

		buf := []byte{7, 'e', 'x'}
		_, err := skipDNSName(buf, 0)

		require.Error(t, err)
	})
}

func TestParseDNSAResponse(t *testing.T) {
	t.Parallel()

	t.Run("too short", func(t *testing.T) {
		t.Parallel()

		_, err := parseDNSAResponse([]byte{0, 0})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "too short")
	})

	t.Run("non-zero rcode", func(t *testing.T) {
		t.Parallel()

		resp := []byte{
			0x00, 0x00, // ID
			0x81, 0x81, // flags + rcode 1
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}

		_, err := parseDNSAResponse(resp)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "rcode")
	})

	t.Run("truncated answer header", func(t *testing.T) {
		t.Parallel()

		query := buildDNSQuery("example.com")
		qLen := questionLength(query)
		resp := make([]byte, 12+qLen+2)
		copy(resp[0:2], query[0:2])
		resp[2] = 0x81
		resp[3] = 0x80
		resp[4] = 0x00
		resp[5] = 0x01
		resp[6] = 0x00
		resp[7] = 0x01
		copy(resp[12:12+qLen], query[12:])

		_, err := parseDNSAResponse(resp)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "truncated at answer header")
	})

	t.Run("successful a record", func(t *testing.T) {
		t.Parallel()

		resp := buildTestDNSResponse("example.com", net.ParseIP("1.2.3.4").To4())

		ips, err := parseDNSAResponse(resp)

		require.NoError(t, err)
		require.Len(t, ips, 1)
		assert.Equal(t, "1.2.3.4", ips[0].String())
	})

	t.Run("multiple a records", func(t *testing.T) {
		t.Parallel()

		resp := buildTestDNSResponseMulti("example.com", []net.IP{
			net.ParseIP("1.2.3.4").To4(),
			net.ParseIP("5.6.7.8").To4(),
		})

		ips, err := parseDNSAResponse(resp)

		require.NoError(t, err)
		require.Len(t, ips, 2)
		assert.Equal(t, "1.2.3.4", ips[0].String())
		assert.Equal(t, "5.6.7.8", ips[1].String())
	})
}

func TestPlainResolver_LookupIP(t *testing.T) {
	t.Parallel()

	t.Run("resolves via plain dns server", func(t *testing.T) {
		t.Parallel()

		// arrange
		addr := startTestDNSServer(t, "example.com", net.ParseIP("9.8.7.6").To4())
		pr := newPlainResolver(addr)

		// act
		ips, err := pr.LookupIP(t.Context(), "ip4", "example.com")

		// assert
		require.NoError(t, err)
		require.Len(t, ips, 1)
		assert.Equal(t, "9.8.7.6", ips[0].String())
	})

	t.Run("dial failure", func(t *testing.T) {
		t.Parallel()

		// arrange
		pr := newPlainResolver("127.0.0.1")

		// act
		_, err := pr.LookupIP(t.Context(), "ip4", "example.com")

		// assert
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dial dns server")
	})
}

func startTestDNSServer(t *testing.T, host string, ip net.IP) string {
	t.Helper()

	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	go func() {
		buf := make([]byte, 512)
		for {
			n, clientAddr, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			resp := buildTestDNSResponseBytes(buf[:n], host, ip)
			_, _ = conn.WriteToUDP(resp, clientAddr)
		}
	}()

	return conn.LocalAddr().String()
}

func buildTestDNSResponse(host string, ip net.IP) []byte {
	return buildTestDNSResponseBytes(buildDNSQuery(host), host, ip)
}

func buildTestDNSResponseMulti(host string, ips []net.IP) []byte {
	query := buildDNSQuery(host)
	qLen := questionLength(query)

	answerSize := 0
	for range ips {
		answerSize += 12 + dnsRDataIPv4 // name pointer(2) + type/class/ttl/rdlen(10) + ip(4)
	}

	resp := make([]byte, 12+qLen+answerSize)
	copy(resp[0:2], query[0:2]) // ID
	resp[2] = 0x81
	resp[3] = 0x80 // flags
	resp[4] = 0x00
	resp[5] = 0x01 // QDCOUNT
	resp[6] = 0x00
	resp[7] = byte(len(ips))           // ANCOUNT
	copy(resp[12:12+qLen], query[12:]) // question

	offset := 12 + qLen
	for _, ip := range ips {
		resp[offset] = 0xC0
		resp[offset+1] = 0x0C // pointer to question name
		offset += 2
		resp[offset] = 0x00
		resp[offset+1] = 0x01 // TYPE A
		offset += 2
		resp[offset] = 0x00
		resp[offset+1] = 0x01 // CLASS IN
		offset += 2
		resp[offset] = 0x00
		resp[offset+1] = 0x00
		resp[offset+2] = 0x00
		resp[offset+3] = 0x3C // TTL
		offset += 4
		resp[offset] = 0x00
		resp[offset+1] = byte(dnsRDataIPv4) // RDLENGTH
		offset += 2
		copy(resp[offset:offset+dnsRDataIPv4], ip)
		offset += dnsRDataIPv4
	}

	return resp
}

func buildTestDNSResponseBytes(query []byte, host string, ip net.IP) []byte {
	_ = host
	qLen := questionLength(query)

	const answerBytes = 16
	resp := make([]byte, 12+qLen+answerBytes)
	copy(resp[0:2], query[0:2])
	resp[2] = 0x81
	resp[3] = 0x80
	resp[4] = 0x00
	resp[5] = 0x01 // QDCOUNT
	resp[6] = 0x00
	resp[7] = 0x01 // ANCOUNT
	copy(resp[12:12+qLen], query[12:])

	offset := 12 + qLen
	resp[offset] = 0xC0
	resp[offset+1] = 0x0C // pointer to question name at offset 12
	offset += 2
	resp[offset] = 0x00
	resp[offset+1] = 0x01 // TYPE A
	offset += 2
	resp[offset] = 0x00
	resp[offset+1] = 0x01 // CLASS IN
	offset += 2
	resp[offset] = 0x00
	resp[offset+1] = 0x00
	resp[offset+2] = 0x00
	resp[offset+3] = 0x3C // TTL
	offset += 4
	resp[offset] = 0x00
	resp[offset+1] = byte(dnsRDataIPv4)
	offset += 2
	copy(resp[offset:offset+dnsRDataIPv4], ip)

	return resp
}

func questionLength(query []byte) int {
	offset := 12
	for {
		if offset >= len(query) {
			return offset - 12 + dnsQuestionTail
		}
		length := int(query[offset])
		offset++
		if length == 0 {
			break
		}
		offset += length
	}
	return offset - 12 + dnsQuestionTail
}
