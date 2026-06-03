package http

import (
	stdhttp "net/http"
	"time"
)

const (
	// defaultTimeout — таймаут HTTP-запроса по умолчанию.
	defaultTimeout = 10 * time.Second
)

// RoundTripper выполняет HTTP-запросы.
type RoundTripper interface {
	RoundTrip(req *stdhttp.Request) (*stdhttp.Response, error)
}

type UserAgentGenerator interface {
	RandomUserAgent() (string, bool)
}

// Option — функциональная опция для настройки HTTP-клиента.
type Option interface {
	apply(*options)
}

type options struct {
	// userAgent — кастомный User-Agent. Если задан — используется вместо ротации.
	userAgent string
	// timeout — таймаут HTTP-запросов.
	timeout time.Duration
	// generator — генератор User-Agent. Если не задан — создаётся по умолчанию.
	generator UserAgentGenerator
	// transport — базовый RoundTripper. Если не задан — используется DefaultTransport.
	transport RoundTripper
}

type userAgentOption struct {
	ua string
}

func (o userAgentOption) apply(opts *options) {
	opts.userAgent = o.ua
}

// WithUserAgent задаёт кастомный User-Agent для всех запросов.
func WithUserAgent(ua string) Option {
	return userAgentOption{ua: ua}
}

type timeoutOption struct {
	d time.Duration
}

func (o timeoutOption) apply(opts *options) {
	opts.timeout = o.d
}

// WithTimeout задаёт таймаут для HTTP-запросов.
func WithTimeout(d time.Duration) Option {
	return timeoutOption{d: d}
}

type generatorOption struct {
	generator UserAgentGenerator
}

func (o generatorOption) apply(opts *options) {
	opts.generator = o.generator
}

// WithUserAgentGenerator задаёт кастомный генератор User-Agent.
func WithUserAgentGenerator(generator UserAgentGenerator) Option {
	return generatorOption{generator: generator}
}

type transportOption struct {
	transport RoundTripper
}

func (o transportOption) apply(opts *options) {
	opts.transport = o.transport
}

// WithTransport задаёт базовый RoundTripper для HTTP-клиента.
func WithTransport(transport RoundTripper) Option {
	return transportOption{transport: transport}
}

// NewClient создаёт HTTP-клиент с настроенной ротацией User-Agent.
// По умолчанию User-Agent выбирается случайным образом из пула популярных браузеров.
func NewClient(opts ...Option) *stdhttp.Client {
	o := options{
		timeout: defaultTimeout,
	}
	for _, opt := range opts {
		opt.apply(&o)
	}

	baseTransport := o.transport
	if baseTransport == nil {
		baseTransport = stdhttp.DefaultTransport
	}

	transport := &userAgentTransport{
		base: baseTransport,
		opts: o,
	}

	return &stdhttp.Client{
		Timeout:   o.timeout,
		Transport: transport,
	}
}

// userAgentTransport добавляет User-Agent к каждому запросу.
type userAgentTransport struct {
	// base — базовый RoundTripper.
	base RoundTripper
	// opts — настройки User-Agent.
	opts options
}

// RoundTrip выполняет HTTP-запрос с подменой User-Agent.
func (t *userAgentTransport) RoundTrip(req *stdhttp.Request) (*stdhttp.Response, error) {
	req = req.Clone(req.Context())

	switch {
	case t.opts.userAgent != "":
		req.Header.Set("User-Agent", t.opts.userAgent)
	case t.opts.generator != nil:
		userAgent, ok := t.opts.generator.RandomUserAgent()
		if ok {
			req.Header.Set("User-Agent", userAgent)
		}
	}

	return t.base.RoundTrip(req)
}
