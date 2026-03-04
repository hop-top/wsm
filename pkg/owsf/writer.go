package owsf

import "sync"

type Writer struct {
	ID  string `json:"id"`
	Seq uint64 `json:"seq"`
}

type WriterRegistry struct {
	mu      sync.Mutex
	writers map[string]*Writer
}

func NewWriterRegistry() *WriterRegistry {
	return &WriterRegistry{
		writers: make(map[string]*Writer),
	}
}

func (r *WriterRegistry) Register(id string) Writer {
	r.mu.Lock()
	defer r.mu.Unlock()
	w := &Writer{ID: id, Seq: 0}
	r.writers[id] = w
	return *w
}

func (r *WriterRegistry) NextSeq(id string) (uint64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	w, ok := r.writers[id]
	if !ok {
		return 0, ErrWriterNotFound
	}
	w.Seq++
	return w.Seq, nil
}

func (r *WriterRegistry) All() []Writer {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Writer, 0, len(r.writers))
	for _, w := range r.writers {
		out = append(out, *w)
	}
	return out
}
