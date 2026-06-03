package useragent

import "math/rand/v2"

type Rand interface {
	IntN(n int) int
}

type Option func(g *Generator)

func WithUserAgentsList(agents []string) Option {
	return func(g *Generator) {
		g.userAgents = agents
	}
}

func WithRand(r Rand) Option {
	return func(g *Generator) {
		g.rand = r
	}
}

// Generator генерирует случайные User-Agent.
type Generator struct {
	// userAgents — пул User-Agent для ротации.
	userAgents []string
	rand       Rand
}

func newDefaultRandV2() Rand {
	return rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
}

// NewGenerator создаёт новый генератор User-Agent.
func NewGenerator(opt ...Option) *Generator {
	gen := &Generator{
		rand: newDefaultRandV2(),
	}

	for _, o := range opt {
		o(gen)
	}

	return gen
}

// RandomUserAgent возвращает случайный User-Agent из пула.
// Второе значение — false, если пул пуст.
func (g *Generator) RandomUserAgent() (string, bool) {
	if len(g.userAgents) == 0 {
		return "", false
	}

	return g.userAgents[g.rand.IntN(len(g.userAgents))], true
}
