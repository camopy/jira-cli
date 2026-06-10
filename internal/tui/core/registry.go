package core

// SectionFactory builds a Section bound to the shared context. Sections are
// constructed lazily on first navigation so unused views cost nothing.
type SectionFactory func(ctx *ProgramContext) Section

// Registry maps section IDs to their factories. It is the single extension
// point: registering a factory makes a view reachable; the App needs no change.
type Registry struct {
	factories map[SectionID]SectionFactory
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{factories: make(map[SectionID]SectionFactory)}
}

// Register associates an ID with a factory, overwriting any previous one.
// It lazily initializes the map so a zero-value Registry is usable.
func (r *Registry) Register(id SectionID, f SectionFactory) {
	if r.factories == nil {
		r.factories = make(map[SectionID]SectionFactory)
	}
	r.factories[id] = f
}

// Has reports whether an ID is registered.
func (r *Registry) Has(id SectionID) bool {
	_, ok := r.factories[id]
	return ok
}

// Build constructs the section for an ID, returning false if it is unregistered.
func (r *Registry) Build(id SectionID, ctx *ProgramContext) (Section, bool) {
	f, ok := r.factories[id]
	if !ok {
		return nil, false
	}
	return f(ctx), true
}
