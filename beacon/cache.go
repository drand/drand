package beacon

import "github.com/drand/kyber/sign"
import "sync"

// PartialCache is a cache storing the partial signatures created & received for
// a given round - avoiding to reconstructing them every time requested.
type PartialCache struct {
	sync.Mutex
	partials map[int][]byte
	expected int
	scheme   sign.ThresholdScheme
}

func NewPartialCache(scheme sign.ThresholdScheme, expected int) *PartialCache {
	return &PartialCache{
		partials: make(map[int][]byte, expected),
		expected: expected,
		scheme:   scheme,
	}
}

func (p *PartialCache) Flush() {
	p.Lock()
	defer p.Unlock()
	p.partials = nil
	p.partials = make(map[int][]byte, p.expected)
}

func (p *PartialCache) Add(partial []byte) {
	p.Lock()
	defer p.Unlock()
	index, err := p.scheme.IndexOf(partial)
	if err != nil {
		// we just don't store it - node will fetch it from network if needed
		return
	}
	p.partials[index] = partial
}

func (p *PartialCache) Len() int {
	p.Lock()
	defer p.Unlock()
	return len(p.partials)
}

func (p *PartialCache) Present(index int) bool {
	p.Lock()
	defer p.Unlock()
	_, present := p.partials[index]
	return present
}

func (p *PartialCache) GetAll() [][]byte {
	p.Lock()
	defer p.Unlock()
	partials := make([][]byte, 0, len(p.partials))
	for _, partial := range p.partials {
		partials = append(partials, partial)
	}
	return partials
}
